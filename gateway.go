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
			actions := service["actions"].(map[string]map[string]interface{})
			for _, action := range actions {
				actionFullName := action["name"].(string)
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

	// setupRoutes is a list of delegates to be invoked when setting up the http server.
	// this allows for other mixins that are combined with the Http gateway
	"setupRoutes": []func(moleculer.BrokerContext, *mux.Router){},

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
}

// populateActionsRouter create a new mux.router
func populateActionsRouter(context moleculer.Context, settings map[string]interface{}, router *mux.Router) (paths []string) {
	if router == nil {
		return paths
	}
	for _, actionHand := range filterActions(context, settings, fetchServices(context)) {
		actionHand.context = context
		path := actionHand.pattern()
		context.Logger().Trace("populateActionsRouter() action -> ", actionHand.action, " path: ", path)
		router.Handle(path, actionHand)
		paths = append(paths, path)
	}
	return paths
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

type GatewayMixin interface {
	// RouterStarting receives a moleculer.context and mux.Router and is expected to publish a route/path using the mux.Router.
	RouterStarting(moleculer.BrokerContext, *mux.Router)
}

type HttpService struct {
	Settings map[string]interface{}
	Mixins   []GatewayMixin
	Deps     []string

	settings      map[string]interface{}
	server        *http.Server
	router        *mux.Router
	actionsRouter *mux.Router
	actionPaths   []string
}

func (svc HttpService) Name() string {
	return "api"
}

func (svc *HttpService) Dependencies() []string {
	return append(svc.Deps, "$node")
}

// createReverseProxy creates a reverse proxy to serve app UI content for ecample on path X and API (gateway content) on path Y.
// used mostly for development.
func (svc *HttpService) createReverseProxy(proxySettings map[string]interface{}) *mux.Router {
	gatewayPath := proxySettings["gatewayPath"].(string)
	target := proxySettings["target"].(string)
	targetPath := proxySettings["targetPath"].(string)

	targetURL, err := url.Parse(target)
	if err != nil {
		panic(errors.New(fmt.Sprint("createReverseProxy() parameter target is invalid. It must be a valid URL! - error: ", err.Error())))
	}
	targetProxy := httputil.NewSingleHostReverseProxy(targetURL)

	fmt.Println("createReverseProxy() handle gatewayPath: ", gatewayPath)
	gatewayRouter := svc.router.PathPrefix(gatewayPath).Subrouter()

	fmt.Println("createReverseProxy() handle targetPath: ", targetPath)
	svc.router.PathPrefix(targetPath).Handler(targetProxy)
	return gatewayRouter
}

func (svc *HttpService) getAddress() string {
	ip := svc.settings["ip"].(string)
	port := svc.settings["port"].(string)
	return fmt.Sprint(ip, ":", port)
}

func (svc *HttpService) reveserProxy(context moleculer.BrokerContext) {
	reverseProxy, hasReverseProxy := svc.settings["reverseProxy"].(map[string]interface{})
	if hasReverseProxy {
		proxySettings := service.MergeSettings(defaultReverseProxy, reverseProxy)
		context.Logger().Debug("Gateway resetHandlers() - reverse proxy enabled - proxySettings: ", proxySettings)
		svc.actionsRouter = svc.createReverseProxy(proxySettings)
	} else {
		svc.actionsRouter = svc.router.PathPrefix("/").Subrouter()
	}
}

func (svc *HttpService) startServer(context moleculer.BrokerContext) {
	address := svc.getAddress()
	context.Logger().Info("Server starting to listen on: ", address)
	err := svc.server.ListenAndServe()
	if err != nil && err.Error() != "http: Server closed" {
		context.Logger().Error("Error listening server on: ", address, " error: ", err)
	}
	context.Logger().Info("Server stopped -> address: ", address)
}

// Started httpService started. It process the settings (default + params), starts a http server,
// notify the plugins that the http server is starting.
func (svc *HttpService) Started(context moleculer.BrokerContext, schema moleculer.ServiceSchema) {
	svc.settings = service.MergeSettings(defaultSettings, schema.Settings, svc.Settings)
	address := svc.getAddress()
	svc.server = &http.Server{Addr: address}
	svc.router = mux.NewRouter()
	svc.server.Handler = svc.router
	for _, mixin := range svc.Mixins {
		mixin.RouterStarting(context, svc.router)
	}
	svc.reveserProxy(context)
	go svc.startServer(context)
	go populateActionsRouter(context.(moleculer.Context), svc.settings, svc.actionsRouter)
	context.Logger().Info("Gateway Started()")
}

func (svc *HttpService) Stopped(context moleculer.BrokerContext, schema moleculer.ServiceSchema) {
	if svc.server != nil {
		err := svc.server.Shutdown(nil)
		if err != nil {
			context.Logger().Error("Error shutting down server - error: ", err)
		}
	}
	context.Logger().Info("Gateway stopped()")
}

// serviceAdded method used to handle the service added event
func (svc *HttpService) serviceAdded(context moleculer.Context, params moleculer.Payload) {
	if svc.actionsRouter == nil {
		return
	}
	go func() {
		paths := populateActionsRouter(context, svc.settings, svc.actionsRouter)
		svc.actionPaths = paths
	}()
}

func (svc *HttpService) ActionPaths() []string {
	return svc.actionPaths
}

func (svc *HttpService) Events() []moleculer.Event {
	return []moleculer.Event{
		{
			Name:    "$registry.service.added",
			Handler: svc.serviceAdded,
		},
	}
}
