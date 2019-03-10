package gateway

import (
	"fmt"
	"net/http"

	"github.com/moleculer-go/moleculer"
	log "github.com/sirupsen/logrus"
)

func createHandler(context moleculer.Context, svc, action map[string]interface{}) func(res http.ResponseWriter, req *http.Request) {
	brokerContext := context.(moleculer.BrokerContext)
	return func(res http.ResponseWriter, req *http.Request) {

		actionName := ""

		context.Call(actionName, params)

		res.Header().Add("X-Request-ID", brokerContext.RequestID())

		//PAREI AQUI ... criar o methodo q mandar external REST/HTTL calls into actions and return results.
	}
}

//mapActions expose actions as REST endpoints
func mapActions(context moleculer.Context) {
	services := <-context.Call("$node.services", map[string]interface{}{
		"onlyAvailable": true,
		"withActions":   true,
	})

	if services.IsError() {
		context.Logger().Error("Could not load the list of services/action from the register. Error: ", services.Error())
		return
	}

	for serviceName, service := range services.MapArray() {
		actions := service["actions"].([]map[string]interface{})
		for _, action := range actions {
			actionName := action["name"].(string)
			path := fmt.Sprint(serviceName, "/", actionName)
			http.HandleFunc(path, createHandler(context, service, action))
		}
	}

}

func serviceCreated(svc moleculer.Service, logger *log.Entry) {
	ip := svc.Settings["ip"].(string)
	port := svc.Settings["port"].(string)
	http.ListenAndServe(fmt.Sprint(ip, ":", port), nil)
}

func serviceStarted(context moleculer.BrokerContext, svc moleculer.Service) {
	mapActions(context.(moleculer.Context))
}

func serviceStopped(context moleculer.BrokerContext, svc moleculer.Service) {

}

func GatewayService() moleculer.Service {
	return moleculer.Service{
		Name: "api",
		Settings: map[string]interface{}{
			// Exposed port
			"port": 3000,

			// Exposed IP
			"ip": "0.0.0.0",
			// Used server instance. If null, it will create a new HTTP(s)(2) server
			// If false, it will start without server in middleware mode
			"server": true,
			// Log the request ctx.params (default to "debug" level)
			"logRequestParams": "debug",

			// Log the response data (default to disable)
			"logResponseData": nil,

			// If set to true, it will log 4xx client errors, as well
			"log4XXResponses": false,

			// Use HTTP2 server (experimental)
			"http2": false,

			// Optimize route order
			"optimizeOrder": true,
		},
		Created: serviceCreated,
		Started: serviceStarted,
		Stopped: serviceStopped,
		Events: []moleculer.Event{
			moleculer.Event{
				Name: "$registry.service.added",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					mapActions(context)
				},
			},
			moleculer.Event{
				Name: "$registry.service.removed",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					mapActions(context)
				},
			},
		},
	}
}
