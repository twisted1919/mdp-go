package core

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type directoryPickup struct {
	Workers          int
	BufferSize       int
	StorageDirectory string

	deliveryServer *deliveryServer
	work           chan *emailFile
	store          *filesStore
}

func (d *directoryPickup) readStorageDirectory() error {
	files, err := ioutil.ReadDir(d.StorageDirectory)
	if err != nil {
		return err
	}

	d.deliveryServer = pickDeliveryServer()
	if d.deliveryServer == nil {
		return nil
	}

	for _, file := range files {
		fileName := file.Name()
		if !strings.HasSuffix(fileName, ".eml") {
			continue
		}
		if !d.store.put(fileName) {
			continue
		}
		eFile := &emailFile{}
		eFile.Path = filepath.Join(d.StorageDirectory, fileName)
		eFile.Name = fileName
		d.work <- eFile
	}
	return nil
}

func (d *directoryPickup) worker() {
	for eFile := range d.work {
		go func(eFile *emailFile) {
			body, err := ioutil.ReadFile(eFile.Path)
			if err != nil {
				d.store.remove(eFile.Name)
				pm(0, fmt.Sprintf("Cannot read file: %s", err))
				return
			}

			go func(body []byte) {
				m := newDeliveryMessage(&body)
				err := d.deliveryServer.forwardMessage(m)
				if err != nil {
					pm(0, fmt.Sprintf("Message ID: %s (%s -> %s): FAILED with: %s.", m.ID, m.getFrom(), m.getTo(), err))
					return
				}
				pm(0, fmt.Sprintf("Message ID: %s (%s -> %s): SUCCESS.", m.ID, m.getFrom(), m.getTo()))
			}(body)

			err = os.Remove(eFile.Path)
			if err != nil {
				os.Rename(eFile.Path, eFile.Path+".deleted")
			}
			d.store.remove(eFile.Name)

		}(eFile)
	}
}

func (d *directoryPickup) start() {
	d.work = make(chan *emailFile, d.BufferSize)
	d.store = &filesStore{
		Store: make(map[string]bool),
	}
	for i := 0; i < d.Workers; i++ {
		go d.worker()
	}
	go func(d *directoryPickup) {
		d.readStorageDirectory()
		ticker := time.NewTicker(time.Second * 1)
		for _ = range ticker.C {
			d.readStorageDirectory()
		}
	}(d)
}

type emailFile struct {
	Path string
	Name string
}

type filesStore struct {
	sync.RWMutex
	Store map[string]bool
}

func (m *filesStore) put(file string) bool {
	m.Lock()
	defer m.Unlock()
	_, exists := m.Store[file]
	if exists {
		return false
	}
	m.Store[file] = true
	return true
}

func (m *filesStore) remove(file string) {
	m.Lock()
	defer m.Unlock()
	delete(m.Store, file)
}

func (m *filesStore) count() int {
	m.RLock()
	defer m.RUnlock()
	return len(m.Store)
}
