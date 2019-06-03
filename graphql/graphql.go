package graphql

import (
	"github.com/moleculer-go/moleculer/payload"
	"strings"

	"github.com/graphql-go/graphql"
	"github.com/moleculer-go/moleculer"
)

type GraphQLService struct {
	brokerContext moleculer.BrokerContext
	serviceSchema moleculer.ServiceSchema
	graphqlSchema graphql.SchemaConfig
}

func (svc *GraphQLService) Events() []moleculer.Event {
	return []moleculer.Event{
		{
			Name:    "$registry.service.added",
			Handler: svc.serviceAdded,
		},
	}
}

// serviceAdded handles when a new service is discovered by the broker
func (svc *GraphQLService) serviceAdded(context moleculer.Context, params moleculer.Payload) {
	svc.rebuildSchema()
}

// rebuildSchema builds the schema
// 1) get all services known to the broker
func (svc *GraphQLService) rebuildSchema() {
	services := []map[string]interface{}{} //TODO fetch services from broker.. see http gateway
	svc.graphqlSchema = svc.buildSchemaConfig(services)
}

//queryActions filter actions and keeps only the ones config for graphql query
func (svc *GraphQLService) queryActions(service map[string]interface{}) (actions map[string]interface{}) {
	payload.New(service).Get("actions").ForEach(func(key interface{}, value Payload) bool{
		if value
		return true
	})

	return actions
}

//mutationActions filter actions and keeps only the ones config for graphql mutation
func (svc *GraphQLService) mutationActions(services []map[string]interface{}) (actions map[string]interface{}) {

	return actions
}

//serviceToQueryFields maps services to graphql fields.
//service must have actions that are graphql query
func (svc *GraphQLService) serviceToQueryFields(services []map[string]interface{}) graphql.Fields {
	fields := graphql.Fields{}
	for _, service := range services {
		actions := svc.queryActions(service)
		if len(actions) == 0 {
			continue
		}
		for k, v := range svc.queryActionsToFields(service, actions) {
			fields[k] = v
		}
	}
	return fields
}

func schemaToGQLFields(obj map[string]interface{}) graphql.Fields {
	fields := graphql.Fields{}

	return fields
}

func schemaToGQLFieldArguments(obj map[string]interface{}) graphql.FieldConfigArgument {
	fields := graphql.FieldConfigArgument{}

	return fields
}

// Args: graphql.FieldConfigArgument{
// 	"fields": &graphql.ArgumentConfig{
// 		Type:        graphql.NewList(graphql.String),
// 		Description: "List fields to populate at the data store.",
// 	},
// 	"populate": &graphql.ArgumentConfig{
// 		Type:        graphql.NewList(graphql.String),
// 		Description: "List fields to populate at the data store.",
// 	},
// },

// actionResolver invoke an action, transform the result and return to resolve handler.
func actionResolver(c moleculer.BrokerContext, action string, transforms ...func(moleculer.Payload) interface{}) graphql.FieldResolveFn {
	if len(transforms) == 0 {
		transforms = append(transforms, func(in moleculer.Payload) interface{} {
			return in.Value()
		})
	}
	return func(p graphql.ResolveParams) (interface{}, error) {
		r := <-c.Call(action, p.Args)
		if r.IsError() {
			return nil, r.Error()
		}
		return transforms[0](r), nil
	}
}

//queryActionToFields maps a single action to graphql fields.
func (svc *GraphQLService) queryActionToFields(service, action map[string]interface{}) (fields graphql.Fields) {
	serviceName := service["name"].(string)
	fullName := action["name"].(string)
	rawName := action["rawName"].(string)

	if rawName == "get" {
		name := "get" + strings.Title(serviceName)
		output := graphql.NewObject(graphql.ObjectConfig{
			Name:   name + "Result",
			Fields: schemaToGQLFields(action["output"].(map[string]interface{})),
		})
		//get a single record. use the name of the service
		fields[name] = &graphql.Field{
			//example service name: user --> field: getUser(id!)
			Name: name,
			Type: output,
			Args: graphql.FieldConfigArgument{
				"id": &graphql.ArgumentConfig{
					Type:        graphql.String,
					Description: "record id to get.",
				},
			},
			Resolve: actionResolver(
				svc.brokerContext,
				fullName),
		}
		return fields
	}

	//list multiple records with filter criteria and pagination.
	if rawName == "list" {
		//example service name: user --> field: listUser(query!), listProperty(query!)
		name := "list" + strings.Title(serviceName)
		output := graphql.NewObject(graphql.ObjectConfig{
			Name:   name + "Result",
			Fields: schemaToGQLFields(action["output"].(map[string]interface{})),
		})
		//use the name of the serviceName + "ById"  Example user + ById = userById
		fields[name] = &graphql.Field{
			Name: name,
			Type: graphql.NewList(output),
			Args: schemaToGQLFieldArguments(action["params"].(map[string]interface{})),
			Resolve: actionResolver(
				svc.brokerContext,
				fullName),
		}
		return fields
	}

	// custom actions
	output := graphql.NewObject(graphql.ObjectConfig{
		Name:   rawName + "Result",
		Fields: schemaToGQLFields(action["output"].(map[string]interface{})),
	})
	fields[rawName] = &graphql.Field{
		Name: rawName,
		Type: graphql.NewList(output),
		Args: schemaToGQLFieldArguments(action["params"].(map[string]interface{})),
		Resolve: actionResolver(
			svc.brokerContext,
			fullName),
	}
	return fields
}

//queryActionsToFields maps multiple actions to graphql fields.
func (svc *GraphQLService) queryActionsToFields(service, actions map[string]interface{}) graphql.Fields {
	fields := graphql.Fields{}
	for _, item := range actions {
		action := item.(map[string]interface{})
		actionFields := svc.queryActionToFields(service, action)
		for k, v := range actionFields {
			fields[k] = v
		}
	}
	return fields
}

func (svc *GraphQLService) mutationFields(services []map[string]interface{}) graphql.Fields {
	fields := graphql.Fields{}

	return fields
}

// buildSchemaConfig give a list of services builds an graphql schema config
func (svc *GraphQLService) buildSchemaConfig(services []map[string]interface{}) graphql.SchemaConfig {
	return graphql.SchemaConfig{
		Query: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Query",
			Fields: svc.serviceToQueryFields(services),
		}),
		Mutation: graphql.NewObject(graphql.ObjectConfig{
			Name:   "Mutation",
			Fields: svc.mutationFields(services),
		}),
	}
}

func (svc *GraphQLService) Started(context moleculer.BrokerContext, schema moleculer.ServiceSchema) {
	svc.brokerContext = context
	svc.serviceSchema = schema
	svc.rebuildSchema()
	context.Logger().Info("GraphQL Gateway Started()")
}

func (svc *GraphQLService) Stopped(context moleculer.BrokerContext, schema moleculer.ServiceSchema) {

	context.Logger().Info("GraphQL Gateway stopped()")
}
