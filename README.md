# mdp-go
Mailwizz Directory Pickup daemon written in GO language

### Quick start     
1. Make sure GO language (https://golang.org/doc/install) is installed on your server.  
2. Clone this repository on your server or download the master archive and unzip it on your server  
3. From command line, navigate inside the downloaded directory/repository   
4. Build the daemon from command line by running ```go build -o mdp-go build.go```  
5. Edit the config.json file and add proper data into it  
6. Make sure no cron job that calls `send-campaigns` command is running
7. Add a delivery server of type Directory Pickup in your Mailwizz APP and make sure the storage directory points to same storage directory as you have set at step 5    
8. Run the daemon from command line using the ```./mdp-go``` command  
9. Create and send a test campaign from your Mailwizz APP  
10. If everything works properly, send it in background using ```nohup ./mdp-go >/dev/null 2>&1 &```

### Notes  
1. Make sure you don't run 2 instances of the daemon that point to same storage directory (`ps aux | grep mdp` should help you)  
2. Make sure you create a delivery server of type Pickup Directory in your Mailwizz APP  
3. This tool is very rudimentary but it gets the job done, if it fails, fix it and send a pull request  

### Configuration  
Most items in configuration are self explanatory, but i'll highlight a few:  
```
DirectoryPickup" : {
	"Workers"          : 128, // the number of processes that run in parallel to read the directory pickup contents
	"BufferSize"       : 32, // the number of emails each worker is allowed to have in the buffer
	"StorageDirectory" : "/path/to/writable/pickup/dir" // same as the one defined in web interface, here mailwizz will write emails and this daemon will read.
},
```  

// this is an "array" of delivery servers used to make the actual delivery for each email from the directory pickup.  
```
"DeliveryServers" : [
	{
		"Hostname"	            : "",
		"Port"		            : 25,
		"Username"	            : "",
		"Password"	            : "",
		"MaxConnections"        : 10, // how many connections to this server to open. connections are reused.
		"MaxConnectionMessages" : 10, // how many emails to send per connection
		"ConnectionTimeout"     : 30,
		"SendRetriesCount"		: 3,
		"TestConnection"		: false,
		"TestRecipientEmail"    : ""
	}
]
```  

// mailwizz has two campaign types, regular and autoresponders, we define settings for both of them.   
```
"Campaigns"   : [
	{
		"Type"      : "regular",
		"Processes" : 10, // how many send-campaigns commands to spawn
		"Limit"     : 3, // how many campaigns each "send-campaigns" command can process
		"Offset"    : 0, // don't play here, no need
		"Pause"     : 2 // how much to sleep between batches
	},
	{
		"Type"      : "autoresponder",
		"Processes" : 5,
		"Limit"     : 3,
		"Offset"    : 0,
		"Pause"     : 2
	}
]```  


### How everything works:  
When you start this daemon it does two main things:  
1. it will start spawning the send-campaigns command from your Mailwizz APP, that is why you need to stop any cron that does this already    
2. it will wait for emails to be written in the pickup directory so that it can start sending them using the servers you defined in config.json file    
When a send-campaigns command is triggered by this daemon, it will choose a delivery server to send your email campaigns.
While the campaign is running, the emails are actually written in the pickup directory instead of sending them directly.
Because the daemon watches the pickup directory, when new emails are added, the daemon will pick them and will use the delivery servers you defined
in the config.json file to do the actual delivery.  
Doing like this we make sure Mailwizz doesn't have to wait for the actual delivery to happen, it's solely purpose is to write the emails in the pickup directory
and then it's this daemon job to send them.
