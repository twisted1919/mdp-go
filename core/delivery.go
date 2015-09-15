package core

import (
	"../smtp"
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"sync"
	"time"
)

type dsConn struct {
	sync.Mutex
	ID            int64
	client        *smtp.Client
	ticker        *time.Ticker
	messagesCount int64
	lastMessageTs int64
	inUse         bool
}

func (conn *dsConn) isInUse() bool {
	conn.Lock()
	defer conn.Unlock()
	return conn.inUse
}

type deliveryServer struct {
	sync.Mutex
	Hostname              string
	Port                  int
	Username              string
	Password              string
	ReturnPath            string
	MaxConnections        int64
	MaxConnectionMessages int64
	ConnectionTimeout     time.Duration
	SendRetriesCount      int
	TestRecipientEmail    string
	TestConnection        bool

	// private attributes
	auth                      smtp.Auth
	calledAuth                bool
	conns                     []dsConn
	connectionsWatcherStarted bool
}

func newDeliveryServer() *deliveryServer {
	server := &deliveryServer{}
	server.startConnectionsWatcher()
	return server
}

func (ds *deliveryServer) getConnectionIdx() int {

	ds.Lock()
	defer ds.Unlock()
	connLength := int64(len(ds.conns))

	// all connection occupied, wait for one to become available
	if connLength >= ds.MaxConnections {
		for {
			for idx, _ := range ds.conns {
				if ds.conns[idx].isInUse() == false {
					return idx
				}
			}
		}
	}

	// not all slots occupied,
	// check existing ones and return one available if possible
	for idx, _ := range ds.conns {
		if ds.conns[idx].isInUse() == false {
			return idx
		}
	}

	// create new connection
	conn := &dsConn{}
	conn.ID = connLength + 1
	ds.conns = append(ds.conns, *conn)

	return int(connLength)
}

func (ds *deliveryServer) startConnectionsWatcher() {
	if ds.connectionsWatcherStarted {
		return
	}
	ds.connectionsWatcherStarted = true
	go func(ds *deliveryServer) {
		ticker := time.NewTicker(time.Second * ds.ConnectionTimeout)
		for _ = range ticker.C {
			// might take time to acquire the lock
			now := time.Now().Unix()
			ds.Lock()
			for idx, _ := range ds.conns {
				if ds.conns[idx].isInUse() {
					continue
				}

				pm(0, fmt.Sprintf("Locking connection #%d", ds.conns[idx].ID))

				// acquire the lock
				ds.conns[idx].Lock()

				tdiff := now - ds.conns[idx].lastMessageTs
				pm(0, fmt.Sprintf("Connection #%d time diff is %d", ds.conns[idx].ID, tdiff))

				if tdiff >= int64(ds.ConnectionTimeout) {
					ds.conns[idx].messagesCount = 0
					ds.conns[idx].lastMessageTs = 0
					if ds.conns[idx].client != nil {
						pm(0, fmt.Sprintf("0. Closing smtp connection #%d", ds.conns[idx].ID))

						ds.conns[idx].client.Quit()
						ds.conns[idx].client.Close()
						ds.conns[idx].client = nil

						dbgMsg := ""
						for i, _ := range ds.conns {
							if ds.conns[i].client != nil {
								dbgMsg += fmt.Sprintf("Connection #%d still open. ", ds.conns[i].ID)
							}
						}
						if dbgMsg != "" {
							pm(0, dbgMsg)
						}
						dbgMsg = ""
					}
				}

				pm(0, fmt.Sprintf("Unlocking connection #%d", ds.conns[idx].ID))

				// release the lock
				ds.conns[idx].Unlock()
			}
			ds.Unlock()
		}
	}(ds)
}

func pickDeliveryServer() *deliveryServer {

	if len(config.DeliveryServers) == 0 {
		return nil
	}

	for idx, server := range config.DeliveryServers {
		for idx2, _ := range server.conns {
			if server.conns[idx2].isInUse() == false {
				config.DeliveryServers[idx].Lock()
				if config.DeliveryServers[idx].calledAuth != true {
					if len(config.DeliveryServers[idx].Username) > 0 || len(config.DeliveryServers[idx].Password) > 0 {
						config.DeliveryServers[idx].auth = smtp.PlainAuth(
							"",
							config.DeliveryServers[idx].Username,
							config.DeliveryServers[idx].Password,
							config.DeliveryServers[idx].Hostname,
						)
					}
					config.DeliveryServers[idx].calledAuth = true
				} else {
					config.DeliveryServers[idx].auth = nil
				}
				config.DeliveryServers[idx].startConnectionsWatcher()
				config.DeliveryServers[idx].Unlock()
				return &config.DeliveryServers[idx]
			}
		}
	}

	config.DeliveryServers[0].Lock()
	if config.DeliveryServers[0].calledAuth != true {
		if len(config.DeliveryServers[0].Username) > 0 || len(config.DeliveryServers[0].Password) > 0 {
			config.DeliveryServers[0].auth = smtp.PlainAuth(
				"",
				config.DeliveryServers[0].Username,
				config.DeliveryServers[0].Password,
				config.DeliveryServers[0].Hostname,
			)
		} else {
			config.DeliveryServers[0].auth = nil
		}
		config.DeliveryServers[0].calledAuth = true
	}
	config.DeliveryServers[0].startConnectionsWatcher()
	config.DeliveryServers[0].Unlock()

	return &config.DeliveryServers[0]
}

func (ds *deliveryServer) forwardMessage(m *deliveryMessage) error {

	if len(m.getFrom()) == 0 || len(m.getTo()) == 0 {
		m = nil
		return errors.New("Cannot find from/to email address")
	}

	mailFrom := ds.Username

	if len(mailFrom) == 0 {
		mailFrom = m.getFrom()
	}

	if len(mailFrom) == 0 && len(m.getReturnPath()) > 0 {
		mailFrom = m.getReturnPath()
	}

	if len(mailFrom) == 0 && len(ds.ReturnPath) > 0 {
		mailFrom = ds.ReturnPath
	}

	idx := ds.getConnectionIdx()
	ds.conns[idx].Lock()
	ds.conns[idx].inUse = true

	if ds.conns[idx].messagesCount >= ds.MaxConnectionMessages {
		ds.conns[idx].messagesCount = 0
		ds.conns[idx].lastMessageTs = 0
		if ds.conns[idx].client != nil {
			pm(0, fmt.Sprintf("1. Closing smtp connection #%d", ds.conns[idx].ID))
			ds.conns[idx].client.Quit()
			ds.conns[idx].client.Close()
			ds.conns[idx].client = nil
		}
	}

	cl := ds.conns[idx].client
	ds.conns[idx].Unlock()

	pm(0, fmt.Sprintf("Message ID: %s (%s -> %s): QUEUED.", m.ID, m.getFrom(), m.getTo()))

	// note: if we include this in the lock, things will get extraordinary slow.
	c, err := smtp.PSendMail(
		fmt.Sprintf("%s:%d", ds.Hostname, ds.Port),
		ds.auth,
		mailFrom,
		[]string{m.getTo()},
		m.Body,
		cl,
	)

	ds.conns[idx].Lock()
	ds.conns[idx].messagesCount++
	ds.conns[idx].lastMessageTs = time.Now().Unix()

	if ds.conns[idx].client == nil {
		pm(0, fmt.Sprintf("Keeping smtp connection #%d open.", ds.conns[idx].ID))
		ds.conns[idx].client = c
	}

	ds.conns[idx].inUse = false
	ds.conns[idx].Unlock()

	// might be a broken pipe, or writing to a closed connection
	// or any other smtp error
	if err != nil {
		if m.retries < ds.SendRetriesCount {
			m.retries++
			pm(0, fmt.Sprintf("Message ID: %s (%s -> %s): RETRY  (%d/%d). Reason: %s", m.ID, m.getFrom(), m.getTo(), m.retries, ds.SendRetriesCount, err))
			ds.conns[idx].Lock()
			ds.conns[idx].messagesCount = 0
			ds.conns[idx].lastMessageTs = 0
			if ds.conns[idx].client != nil {
				pm(0, fmt.Sprintf("2. Closing smtp connection #%d.", ds.conns[idx].ID))
				ds.conns[idx].client.Quit()
				ds.conns[idx].client.Close()
				ds.conns[idx].client = nil
			}
			ds.conns[idx].Unlock()
			time.Sleep(time.Second * time.Duration(m.retries))
			return ds.forwardMessage(m)
		} else {
			pm(0, fmt.Sprintf("Message ID: %s (%s -> %s): GIVING UP  (%d/%d). Reason: %s", m.ID, m.getFrom(), m.getTo(), m.retries, ds.SendRetriesCount, err))
		}
	}

	m = nil
	return err
}

func (ds *deliveryServer) emailTest() error {
	if ds.TestConnection != true || len(ds.TestRecipientEmail) == 0 {
		return nil
	}

	ds.auth = smtp.PlainAuth(
		"",
		ds.Username,
		ds.Password,
		ds.Hostname,
	)

	if len(ds.Username) > 0 || len(ds.Password) > 0 {
		ds.auth = nil
	}

	from := mail.Address{"Mailwizz", ds.Username}
	to := mail.Address{"Mailwizz Owner", ds.TestRecipientEmail}
	subject := "Test sent from mailwizz binary"
	body := "Test sent from mailwizz binary"

	header := make(map[string]string)
	header["From"] = from.String()
	header["To"] = to.String()
	header["Subject"] = ds.encodeRFC2047(subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	return smtp.SendMail(
		fmt.Sprintf("%s:%d", ds.Hostname, ds.Port),
		ds.auth,
		ds.Username,
		[]string{to.Address},
		[]byte(message),
	)
}

func deliveryServersConnectionTests() {
	fmt.Println("Testing delivery servers connectivity...")
	failure := false
	for _, ds := range config.DeliveryServers {
		fmt.Print(fmt.Sprintf("Checking %s(%s)... ", ds.Hostname, ds.Username))
		if ds.TestConnection != true || len(ds.TestRecipientEmail) == 0 {
			fmt.Print("testing is not enabled.\n")
			continue
		}
		err := ds.emailTest()
		if err != nil {
			fmt.Printf("testing failed with: %s.\n", err)
			failure = true
		} else {
			fmt.Print("testing passed.\n")
		}
	}
	if failure {
		log.Fatal("Cannot proceed with unusable delivery servers, please recheck your configuration file!")
	}
}

func (ds *deliveryServer) encodeRFC2047(String string) string {
	addr := mail.Address{String, ""}
	return strings.Trim(addr.String(), " <>")
}

type deliveryMessage struct {
	ID         string
	Body       []byte
	from       string
	to         string
	returnPath string
	retries    int
	// flags
	fromParsed       bool
	toParsed         bool
	returnPathParsed bool
	// mail message containing Headers and Body
	mailMessage *mail.Message
}

func newDeliveryMessage(body *[]byte) *deliveryMessage {
	dm := &deliveryMessage{}
	dm.Body = *body
	body = nil
	if len(dm.Body) > 0 {
		mm, err := mail.ReadMessage(bytes.NewReader(dm.Body))
		if err == nil {
			dm.mailMessage = mm
		}
	}
	return dm
}

func (dm *deliveryMessage) getFrom() string {
	if dm.fromParsed {
		return dm.from
	}
	dm.fromParsed = true
	if len(dm.from) > 0 {
		return dm.from
	}
	if dm.mailMessage == nil {
		return ""
	}
	addr, err := mail.ParseAddress(dm.mailMessage.Header.Get("From"))
	if err != nil {
		return ""
	}
	dm.from = addr.Address
	return dm.from
}

func (dm *deliveryMessage) getTo() string {
	if dm.toParsed {
		return dm.to
	}
	dm.toParsed = true
	if len(dm.to) > 0 {
		return dm.to
	}
	if dm.mailMessage == nil {
		return ""
	}
	addr, err := mail.ParseAddress(dm.mailMessage.Header.Get("To"))
	if err != nil {
		return ""
	}
	dm.to = addr.Address
	return dm.to
}

func (dm *deliveryMessage) getReturnPath() string {
	if dm.returnPathParsed {
		return dm.returnPath
	}
	dm.returnPathParsed = true
	if len(dm.returnPath) > 0 {
		return dm.returnPath
	}
	if dm.mailMessage == nil {
		return ""
	}
	addr, err := mail.ParseAddress(dm.mailMessage.Header.Get("Return-Path"))
	if err != nil {
		return ""
	}
	dm.returnPath = addr.Address
	return dm.returnPath
}
