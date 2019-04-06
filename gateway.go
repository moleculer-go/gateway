package gateway

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/serializer"
	"github.com/moleculer-go/moleculer/service"
	log "github.com/sirupsen/logrus"
)

var jsonSerializer = serializer.CreateJSONSerializer(log.WithField("gateway", "json-serializer"))

var actionWildCardRegex = regexp.MustCompile(`(.+)\.\*`)
var serviceWildCardRegex = regexp.MustCompile(`\*\.(.+)`)
var serviceActionRegex = regexp.MustCompile(`(.+)\.(.+)`)

//shouldInclude check if the actions should be added based on the whitelist.
func shouldInclude(whitelist []string, action string) bool {
	for _, item := range whitelist {
		if item == "**" || item == "*.*" {
			return true
		}
		whitelistService := actionWildCardRegex.FindStringSubmatch(item)
		if len(whitelistService) > 0 && whitelistService[1] != "" {
			actionService := serviceActionRegex.FindStringSubmatch(action)
			if len(actionService) > 1 && len(whitelistService) > 1 && actionService[1] == whitelistService[1] {
				return true
			}
		}
		whitelistAction := serviceWildCardRegex.FindStringSubmatch(item)
		if len(whitelistAction) > 0 && whitelistAction[1] != "" {
			actionName := serviceActionRegex.FindStringSubmatch(action)
			if len(actionName) > 2 && len(whitelistAction) > 1 && actionName[2] == whitelistAction[1] {
				return true
			}
		}
		itemRegex, err := regexp.Compile(item)
		if err == nil {
			if itemRegex.MatchString(action) {
				return true
			}
		}
	}
	return false
}

var validMethods = []string{"GET", "POST", "PUT", "DELETE"}

func validMethod(method string) bool {
	for _, item := range validMethods {
		if item == method {
			return true
		}
	}
	return false
}

func paramsFromRequestForm(request *http.Request, logger *log.Entry) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	err := request.ParseForm()
	if err != nil {
		logger.Error("Error calling request.ParseForm() -> ", err)
		return nil, err
	}
	for name, value := range request.Form {
		if len(value) == 1 {
			params[name] = value[0]
		} else {
			params[name] = value
		}
	}
	return params, nil
}

// paramsFromRequest extract params from body and URL into a payload.
func paramsFromRequest(request *http.Request, logger *log.Entry) moleculer.Payload {
	mvalues, err := paramsFromRequestForm(request, logger)
	if len(mvalues) > 0 {
		return payload.New(mvalues)
	}
	if err != nil {
		return payload.Error("Error trying to parse request form values. Error: ", err.Error())
	}

	bts, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return payload.Error("Error trying to parse request body. Error: ", err.Error())
	}
	return jsonSerializer.BytesToPayload(&bts)
}

func invertStringMap(in map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range in {
		out[value] = key
	}
	return out
}

//createActionHandlers create actionHanler for each action with the prefixPath.
func createActionHandlers(route map[string]interface{}, actions []string) []*actionHandler {
	routePath := route["path"].(string)
	mappingPolicy, exists := route["mappingPolicy"].(string)
	if !exists {
		mappingPolicy = "all"
	}
	aliases, exists := route["aliases"].(map[string]string)
	if !exists {
		aliases = map[string]string{}
	}
	actionToAlias := invertStringMap(aliases)

	result := []*actionHandler{}
	for _, action := range actions {
		actionAlias, exists := actionToAlias[action]
		if !exists && mappingPolicy == "restrict" {
			continue
		}
		result = append(result, &actionHandler{alias: actionAlias, routePath: routePath, action: action})
	}
	return result
}

// fetchServices fetch the services and actions that will be exposed.
func fetchServices(context moleculer.Context) []map[string]interface{} {
	services := <-context.Call("$node.services", map[string]interface{}{
		"onlyAvailable": true,
		"withActions":   true,
	})
	if services.IsError() {
		context.Logger().Error("Could not load the list of services/action from the register. Error: ", services.Error())
		return []map[string]interface{}{}
	}
	return services.MapArray()
}

//filterActions with a list of services collect all actions, applyfilter based on
// whitelist settings and create action handlers for each action.
func filterActions(context moleculer.Context, settings map[string]interface{}, services []map[string]interface{}) []*actionHandler {
	result := []*actionHandler{}
	routes := settings["routes"].([]map[string]interface{})
	for _, route := range routes {
		filteredActions := []string{}
		_, exists := route["whitelist"]
		whitelist := []string{"**"}
		if exists {
			whitelist = route["whitelist"].([]string)
		}
		for _, service := range services {
			actions := service["actions"].([]map[string]interface{})
			for _, action := range actions {
				actionFullName := fmt.Sprint(service["name"].(string), ".", action["name"].(string))
				if shouldInclude(whitelist, actionFullName) {
					filteredActions = append(filteredActions, actionFullName)
				}
			}
		}
		for _, actionHand := range createActionHandlers(route, filteredActions) {
			result = append(result, actionHand)
		}
	}
	return result
}

//onErrorHandler default error handler.
func onErrorHandler() {
	//TODO
}

var defaultRoutes = []map[string]interface{}{
	map[string]interface{}{
		"path": "/",

		//whitelist filter used to filter the list of actions.
		//accept regex, and wildcard on action name
		//regex: /^math\.\w+$/
		//wildcard: posts.*
		"whitelist": []string{"**"},

		//mappingPolicy -> all : include all actions, the ones with aliases and without.
		//mappingPolicy -> restrict : include only actions that are in the list of aliases.
		"mappingPolicy": "all",

		//aliases -> alias names instead of action names.
		// "aliases": map[string]interface{}{
		// 	"login": "auth.login"
		// },

		//authorization turn on/off authorization
		"authorization": false,
	},
}

var defaultSettings = map[string]interface{}{

	// reverseProxy define a reverse proxy for local development and avoid CORS issues :)
	"reverseProxy": false,

	// socket.io path where the server will listen for websocket connections.
	// when set enables mapping of moleculer events to socket.io events.
	"socket.io": "/ws/",

	// Exposed port
	"port": "3100",

	// Exposed IP
	"ip": "0.0.0.0",

	// Used server instance. If null, it will create a new HTTP(s)(2) server
	// If false, it will start without server in middleware mode
	//"server": true,

	// Log the request ctx.params (default to "debug" level)
	"logRequestParams": "debug",

	// Log the response data (default to disable)
	"logResponseData": nil,

	// If set to true, it will log 4xx client errors, as well
	"log4XXResponses": false,

	// Use HTTP2 server (experimental)
	//"http2": false,

	// Optimize route order
	"optimizeOrder": true,

	//routes
	"routes": defaultRoutes,

	"assets": map[string]interface{}{
		"folder":  "./www",
		"options": map[string]interface{}{
			//options for static module
		},
	},

	"onError": onErrorHandler,
}

// populateActionsRouter create a new mux.router
func populateActionsRouter(context moleculer.Context, settings map[string]interface{}, router *mux.Router) {
	if router == nil {
		return
	}
	for _, actionHand := range filterActions(context, settings, fetchServices(context)) {
		actionHand.context = context
		path := actionHand.pattern()
		context.Logger().Debug("populateActionsRouter() action -> ", actionHand.action, " path: ", path)
		router.Handle(path, actionHand)
	}
}

// when enable these are the default values
var defaultReverseProxy = map[string]interface{}{
	//gateway endpoint path
	"gatewayPath": "/api",

	//reserse proxy target ip:port
	"target": "http://localhost:3000",
	//reserse proxy path
	"targetPath": "/",
}

// createReverseProxy creates a reverse proxy to serve app UI content for ecample on path X and API (gateway content) on path Y.
// used mostly for development.
func createReverseProxy(proxySettings map[string]interface{}, instance *moleculer.Service, routes *mux.Router) *mux.Router {
	gatewayPath := proxySettings["gatewayPath"].(string)
	target := proxySettings["target"].(string)
	targetPath := proxySettings["targetPath"].(string)

	targetUrl, err := url.Parse(target)
	if err != nil {
		panic(errors.New(fmt.Sprint("createReverseProxy() parameter target is invalid. It must be a valid URL! - error: ", err.Error())))
	}
	targetProxy := httputil.NewSingleHostReverseProxy(targetUrl)

	fmt.Println("createReverseProxy() handle gatewayPath: ", gatewayPath)
	gatewayRouter := routes.PathPrefix(gatewayPath).Subrouter()

	fmt.Println("createReverseProxy() handle targetPath: ", targetPath)
	routes.PathPrefix(targetPath).Handler(targetProxy)
	return gatewayRouter
}

func getAddress(instance *moleculer.Service) string {
	ip := instance.Settings["ip"].(string)
	port := instance.Settings["port"].(string)
	return fmt.Sprint(ip, ":", port)
}

// socketServer checks service settings and if enabled create an socket.io server.
func socketServer(context moleculer.BrokerContext, instance *moleculer.Service) (*WebSocketPubSub, string) {
	if instance.Settings["socket.io"] != nil && instance.Settings["socket.io"] != "" {
		path, ok := instance.Settings["socket.io"].(string)
		if ok {
			context.Logger().Debug("socket.io settings found -> binding socket.io on path: ", path)
			return NewWebSocketPubSub(context), path
		}
	}
	return nil, ""
}

//Service create the service schema for the API Gateway service.
func Service(settings ...map[string]interface{}) moleculer.Service {
	var instance *moleculer.Service
	allSettings := []map[string]interface{}{defaultSettings}
	for _, set := range settings {
		if set != nil {
			allSettings = append(allSettings, set)
		}
	}
	serviceSettings := service.MergeSettings(allSettings...)
	var server *http.Server
	var actionsRouter *mux.Router
	rootRouter := mux.NewRouter()

	serviceStarted := func(context moleculer.BrokerContext, svc moleculer.Service) {
		instance = &svc
		address := getAddress(instance)
		server = &http.Server{Addr: address}
		socketServer, socketPath := socketServer(context, instance)
		if socketServer != nil {
			rootRouter.Handle(socketPath, socketServer.Handler())
		}
		reverseProxy, hasReverseProxy := instance.Settings["reverseProxy"].(map[string]interface{})
		if hasReverseProxy {
			proxySettings := service.MergeSettings(defaultReverseProxy, reverseProxy)
			context.Logger().Debug("Gateway resetHandlers() - reverse proxy enabled - proxySettings: ", proxySettings)
			actionsRouter = createReverseProxy(proxySettings, instance, rootRouter)
		} else {
			actionsRouter = rootRouter.PathPrefix("/").Subrouter()
		}
		server.Handler = rootRouter
		go (func() {
			context.Logger().Info("Server starting to listen on: ", address)
			err := server.ListenAndServe()
			if err != nil && err.Error() != "http: Server closed" {
				context.Logger().Error("Error listening server on: ", address, " error: ", err)
			}
			context.Logger().Info("Server stopped -> address: ", address)
		})()
		go populateActionsRouter(context.(moleculer.Context), instance.Settings, actionsRouter)
	}

	serviceStoped := func(context moleculer.BrokerContext, svc moleculer.Service) {
		context.Logger().Info("Gateway stopped()")
		if server != nil {
			err := server.Shutdown(nil)
			if err != nil {
				context.Logger().Error("Error shutting down server - error: ", err)
			}
		}
	}

	return moleculer.Service{
		Name:         "api",
		Settings:     serviceSettings,
		Dependencies: []string{"$node"},
		Started:      serviceStarted,
		Stopped:      serviceStoped,
		Events: []moleculer.Event{
			moleculer.Event{
				Name: "$registry.service.added",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					if instance == nil || actionsRouter == nil {
						return
					}
					go populateActionsRouter(context.(moleculer.Context), instance.Settings, actionsRouter)
				},
			},
			moleculer.Event{
				Name: "$registry.service.removed",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					// At the moment populateActionsRouter cannot remove routes :(
					// so it does make sense to call it when a service is removed.
					//the route will be there.. and moleculer will reject the call
					//go populateActionsRouter(context.(moleculer.Context), instance.Settings, actionsRouter)
				},
			},
		},
	}
}
