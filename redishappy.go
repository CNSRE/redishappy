package main

import (
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
	"github.com/mdevilliers/redishappy/configuration"
	"github.com/mdevilliers/redishappy/sentinel"
	"github.com/mdevilliers/redishappy/services/flipper"
	"github.com/mdevilliers/redishappy/services/logger"
	"github.com/mdevilliers/redishappy/util"
	"net/http"
)

func main() {

	//TODO : configure from command line
	logPath := "log" //var/log/redis-happy")
	configFile := "config.json"

	logger.InitLogging(logPath)

	logger.Info.Print("redis-happy started")

	configuration, err := configuration.LoadFromFile(configFile)

	if err != nil {
		logger.Error.Panic(err)
	}

	logger.Info.Printf("Parsed from config : %s\n", util.String(configuration))

	sentinelManager := sentinel.NewManager()

	go startMonitoring(sentinelManager, configuration)

	initApiServer()
}

func initApiServer() {

	logger.Info.Print("hosting json endpoint.")
	service := rpc.NewServer()
	service.RegisterCodec(json.NewCodec(), "application/json")
	service.RegisterService(new(HelloService), "")
	http.Handle("/rpc", service)
	http.ListenAndServe(":8085", nil)
}

func startMonitoring(sentinelManager *sentinel.SentinelManager, configuration *configuration.Configuration) {

	flipper := flipper.New(configuration)
	switchmasterchannel := make(chan sentinel.MasterSwitchedEvent)
	go loopSentinelEvents(flipper, switchmasterchannel)

	started := 0

	for _, configuredSentinel := range configuration.Sentinels {

		_, err := sentinelManager.StartMonitoring(configuredSentinel)

		if err != nil {

			logger.Info.Printf("Error starting sentinel (%s) healthchecker : %s", configuredSentinel.GetLocation(), err.Error())

		} else {

			started++

			pubsubclient, err := sentinel.NewPubSubClient(configuredSentinel)

			if err != nil {
				logger.Info.Printf("Error starting sentinel (%s) monitor : %s", configuredSentinel.GetLocation(), err.Error())
			}

			pubsubclient.StartMonitoringMasterEvents(switchmasterchannel)
		}
	}

	if started == 0 {
		logger.Info.Printf("WARNING : no sentinels are currently being monitored.")
	}
}

func loopSentinelEvents(flipper *flipper.FlipperClient, switchmasterchannel chan sentinel.MasterSwitchedEvent) {

	for switchEvent := range switchmasterchannel {
		flipper.Orchestrate(switchEvent)
	}
}

type HelloArgs struct {
	Who string
}

type HelloReply struct {
	Message string
}

type HelloService struct{}

func (h *HelloService) Say(r *http.Request, args *HelloArgs, reply *HelloReply) error {
	reply.Message = "Hello, " + args.Who + "!"
	return nil
}
