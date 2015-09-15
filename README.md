# mdp-go
Mailwizz Directory Pickup daemon written in GO language

### Quick start     
1. Make sure GO language (https://golang.org/doc/install) is installed on your server.  
2. Clone this repository on your server or download the master archive and unzip it on your server  
3. From command line, navigate inside the downloaded directory/repo   
4. Build the daemon from command line with ```go build -o mdp-go build.go```  
5. Edit the config.json file and add proper data into it  
6. Make sure no cron job that calls `send-campaigns` command is running  
7. Run the daemon from command line using the ```./mdp-go``` command  
8. If everything works properly, send it in background using ```nohup ./mdp-go >/dev/null 2>&1 &```

### Notes  
Make sure you don't run 2 instances of the daemon that point to same storage directory.
