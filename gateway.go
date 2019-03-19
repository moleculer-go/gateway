package gateway

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/moleculer-go/moleculer/payload"

	"github.com/moleculer-go/moleculer"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"
)

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

type actionHandler struct {
	routePath            string
	alias                string
	action               string
	context              moleculer.Context
	acceptedMethodsCache map[string]bool
}

// aliasPath return the alias path, if one exists for the action.
func (handler *actionHandler) aliasPath() string {
	if handler.alias != "" {
		parts := strings.Split(strings.TrimSpace(handler.alias), " ")
		alias := ""
		if len(parts) == 1 {
			alias = parts[0]
		} else if len(parts) == 2 {
			alias = parts[1]
		} else {
			panic(fmt.Sprint("Invalid alias format! -> ", handler.alias))
		}
		return alias
	}
	return ""
}

// pattern return the path pattern used to map URL in the http.ServeMux
func (handler *actionHandler) pattern() string {
	actionPath := strings.Replace(handler.action, ".", "/", -1)
	fullPath := ""
	aliasPath := handler.aliasPath()
	if aliasPath != "" {
		fullPath = fmt.Sprint(handler.routePath, "/", aliasPath)
	} else {
		fullPath = fmt.Sprint(handler.routePath, "/", actionPath)
	}
	return strings.Replace(fullPath, "//", "/", -1)
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

//acceptedMethods return a map of accepted methods for this handler.
func (handler *actionHandler) acceptedMethods() map[string]bool {
	if handler.acceptedMethodsCache != nil {
		return handler.acceptedMethodsCache
	}
	if handler.alias != "" {
		parts := strings.Split(strings.TrimSpace(handler.alias), " ")
		if len(parts) == 2 {
			method := strings.ToUpper(parts[0])
			if validMethod(method) {
				handler.acceptedMethodsCache = map[string]bool{
					method: true,
				}
				return handler.acceptedMethodsCache
			}
		}
	}
	handler.acceptedMethodsCache = map[string]bool{
		"GET":    true,
		"POST":   true,
		"PUT":    true,
		"DELETE": true,
	}
	return handler.acceptedMethodsCache
}

// invalidHttpMethodError send an error in the reponse about the http method being invalid.
func invalidHttpMethodError(response http.ResponseWriter, methods map[string]bool) {
	acceptedMethods := []string{}
	for methodName := range methods {
		acceptedMethods = append(acceptedMethods, methodName)
	}
	error := fmt.Errorf("Invalid HTTP Method - accepted methods: %s", acceptedMethods)
	rChan := make(chan moleculer.Payload, 1)
	rChan <- payload.Create(error)
	sendReponse(rChan, response)
}

var succesStatusCode = 200
var errorStatusCode = 500
var resultParseErrorStatusCode = 500

// sendReponse send the result payload  back using the ResponseWriter
func sendReponse(resultChan chan moleculer.Payload, response http.ResponseWriter) {
	result := <-resultChan

	json := []byte{}
	errors := []string{}
	result.ForEach(func(key interface{}, value moleculer.Payload) bool {
		var err error
		if key == nil && value.IsError() {
			json, err = sjson.SetBytes(json, "error", value.Error().Error())
			if err != nil {
				errors = append(errors, fmt.Sprint("Error trying to parse the value of field:", key, " Error: ", err))
			}
		} else if key == nil {
			json = []byte(fmt.Sprint(value.Value()))
		} else {
			json, err = sjson.SetBytes(json, key.(string), value.Value())
			if err != nil {
				errors = append(errors, fmt.Sprint("Error trying to parse the value of field:", key, " Error: ", err))
			}
		}
		return true
	})

	if len(errors) > 0 {
		json = []byte{}
		for _, item := range errors {
			json, _ = sjson.SetBytes(json, "errors.-1", item)
		}
		response.Write(json)
		response.WriteHeader(resultParseErrorStatusCode)
		return
	}
	_, err := response.Write(json)
	if err != nil {
		panic(fmt.Sprint("Could not send reponse! error: ", err))
	}
	if result.IsError() {
		response.WriteHeader(errorStatusCode)
	} else {
		response.WriteHeader(succesStatusCode)
	}
}

// paramsFromRequest extract params from body and URL into a payload.
func paramsFromRequest(request *http.Request, logger *log.Entry) moleculer.Payload {
	params := map[string]interface{}{}
	err := request.ParseForm()
	if err != nil {
		logger.Error("Error calling request.ParseForm() -> ", err)
		return payload.Create(err)
	}
	for name, value := range request.Form {
		if len(value) == 1 {
			params[name] = value[0]
		} else {
			params[name] = value
		}
	}
	return payload.Create(params)
}

func (handler *actionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	methods := handler.acceptedMethods()
	logger := handler.context.Logger()
	switch request.Method {
	case http.MethodGet:
		if methods["GET"] {
			sendReponse(handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodPost:
		if methods["POST"] {
			sendReponse(handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodPut:
		if methods["PUT"] {
			sendReponse(handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	case http.MethodDelete:
		if methods["DELETE"] {
			sendReponse(handler.context.Call(handler.action, paramsFromRequest(request, logger)), response)
		}
	default:
		invalidHttpMethodError(response, methods)
	}
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
func filterActions(settings map[string]interface{}, services []map[string]interface{}) []*actionHandler {
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
	// Exposed port
	"port": "3000",

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

//createServeMux create a new http.ServeMux
func createServeMux(settings map[string]interface{}, context moleculer.Context) *http.ServeMux {
	serveMux := http.NewServeMux()
	for _, actionHand := range filterActions(settings, fetchServices(context)) {
		actionHand.context = context
		serveMux.Handle(actionHand.pattern(), actionHand)
	}
	return serveMux
}

//Service create the service schema for the API Gateway service.
func Service() moleculer.Service {

	var instanceSettings = defaultSettings
	var server *http.Server

	return moleculer.Service{
		Name:     "api",
		Settings: defaultSettings,
		Created: func(svc moleculer.Service, logger *log.Entry) {
			ip := svc.Settings["ip"].(string)
			port := svc.Settings["port"].(string)
			address := fmt.Sprint(ip, ":", port)
			server = &http.Server{Addr: address}
			logger.Info("Gateway starting server on: ", address)
			err := server.ListenAndServe()
			if err != nil && err.Error() != "http: Server closed" {
				logger.Error("Error listening server on: ", address, " error: ", err)
			}
			logger.Info("Server stoped -> address: ", address)
		},
		Started: func(context moleculer.BrokerContext, svc moleculer.Service) {
			instanceSettings = svc.Settings
			server.Handler = createServeMux(instanceSettings, context.(moleculer.Context))
		},
		Stopped: func(context moleculer.BrokerContext, svc moleculer.Service) {
			context.Logger().Info("Gateway stopped()")
			err := server.Shutdown(nil)
			if err != nil {
				context.Logger().Error("Error shutting down server - error: ", err)
			}
		},
		Events: []moleculer.Event{
			moleculer.Event{
				Name: "$registry.service.added",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					server.Handler = createServeMux(instanceSettings, context.(moleculer.Context))
				},
			},
			moleculer.Event{
				Name: "$registry.service.removed",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					server.Handler = createServeMux(instanceSettings, context.(moleculer.Context))
				},
			},
		},
	}
}
