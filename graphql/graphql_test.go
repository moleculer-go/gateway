package graphql

import (
	"fmt"
	"github.com/moleculer-go/moleculer/payload"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Graphql", func() {

	//mutation example: user event store
	//this will create the follow types in the graph
	// - Mutation
	//  - userEventStore
	// 	 - create
	mutationExample := map[string]interface{}{
		"name": "userEventStore",
		"actions": map[string]map[string]interface{}{
			"userEventStore.create": map[string]interface{}{
				"name":    "userEventStore.create",
				"rawName": "create",
				"params": map[string]interface{}{
					"name":     map[string]interface{}{"type": "string", "optional": false},
					"lastname": map[string]interface{}{"type": "string", "optional": true},
				},
				"output": map[string]interface{}{
					"eventId":   map[string]interface{}{"type": "number", "integer": true},
					"createdAt": map[string]interface{}{"type": "number", "integer": true},
				},
				"graphql": "mutation",
				"metrics": map[string]interface{}{},
			},
		},
	}

	userOutput := map[string]interface{}{
		"name":     map[string]interface{}{"type": "string"},
		"lastname": map[string]interface{}{"type": "string"},
		"age":      map[string]interface{}{"type": "number", "integer": true},
	}

	//query example: user aggregate find action. return a paginated list of users.
	// can query for simple query+ query field or with complex queries using the query field.
	// - Query
	//  - user -> maps to user.get
	// 	- users -> maps to user.find
	// 	- expiredUsers -> maps to user.expiredUsers
	queryExample := map[string]interface{}{
		"name": "user",
		"actions": map[string]map[string]interface{}{
			// -> will map to query/user and  query/usersByIds for multiple ids
			"user.get": map[string]interface{}{
				"name":    "user.get",
				"rawName": "get",
				"params": map[string]interface{}{
					"ids":      map[string]interface{}{"type": "array", "items": "string"},
					"fields":   map[string]interface{}{"type": "array", "items": "string", "optional": true},
					"populate": map[string]interface{}{"type": "array", "items": "string", "optional": true},
					"mapping":  map[string]interface{}{"type": "boolean", "optional": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": map[string]interface{}{},
			},
			// -> will map to query/users
			"user.list": map[string]interface{}{
				"name":    "user.list",
				"rawName": "list",
				"params": map[string]interface{}{
					"query":        map[string]interface{}{"type": "object", "optional": true},
					"search":       map[string]interface{}{"type": "string", "optional": true},
					"searchFields": map[string]interface{}{"type": "array", "items": "string", "optional": true},
					"fields":       map[string]interface{}{"type": "array", "items": "string", "optional": true},
					"populate":     map[string]interface{}{"type": "array", "items": "string", "optional": true},
					"sort":         map[string]interface{}{"type": "string", "optional": true},
					"page":         map[string]interface{}{"type": "number", "integer": true, "optional": true},
					"pageSize":     map[string]interface{}{"type": "number", "integer": true, "optional": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": map[string]interface{}{},
			},
			//custom action query/expiredUsers
			"user.expiredUsers": map[string]interface{}{
				"name":    "user.expiredUsers",
				"rawName": "expiredUsers",
				"params": map[string]interface{}{
					"periodFrom": map[string]interface{}{"type": "number", "integer": true},
					"periodTo":   map[string]interface{}{"type": "number", "integer": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": map[string]interface{}{},
			},
		},
	}

	FIt("queryActionsToFields should create graphql fields based on action definitions ", func() {
		svc := GraphQLService{}
		p := payload.New(queryExample)
		fields := svc.queryActionsToFields(queryExample, p.Get("actions").RawMap())
		fmt.Println("queryActionsToFields fields")
		fmt.Println(fields)
		Expect(fields).ShouldNot(BeNil())
		Expect(fields["UserById"]).ShouldNot(BeNil())
		Expect(fields["ListUser"]).ShouldNot(BeNil())
		Expect(fields["ExpiredUsers"]).ShouldNot(BeNil())

	})

	It("should build a schema config", func() {
		services := []map[string]interface{}{mutationExample, queryExample}

		svc := GraphQLService{}
		graphSchema := svc.buildSchemaConfig(services)

		Expect(graphSchema).ShouldNot(BeNil())
	})

	It("mapToGraphqlObject should build graphql object", func() {
		services := []map[string]interface{}{mutationExample, queryExample}

		svc := GraphQLService{}
		graphSchema := svc.buildSchemaConfig(services)

		Expect(graphSchema).ShouldNot(BeNil())
	})

	It("queryActions should return actions that are graphql query", func() {
		svc := GraphQLService{}
		queryExample["user.nonGrapQl"] = map[string]interface{}{
			"name":    "user.nonGrapQl",
			"rawName": "nonGrapQl",
			"params": map[string]interface{}{
				"ids": map[string]interface{}{"type": "array", "items": "string"},
			},
			"metrics": map[string]interface{}{},
		}
		actions := svc.queryActions(queryExample)
		Expect(len(actions)).Should(Equal(3))
	})
})
