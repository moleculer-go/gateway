package graphql

import (
	"github.com/moleculer-go/moleculer/payload"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type M map[string]interface{}

var _ = Describe("Graphql", func() {

	//mutation example: user event store
	//this will create the follow types in the graph
	// - Mutation
	//  - userEventStore
	// 	 - create
	mutationExample := M{
		"name": "userEventStore",
		"actions": map[string]M{
			"userEventStore.create": M{
				"name":    "userEventStore.create",
				"rawName": "create",
				"params": M{
					"name":     M{"type": "string", "optional": false},
					"lastname": M{"type": "string", "optional": true},
				},
				"output": M{
					"eventId":   M{"type": "number", "integer": true},
					"createdAt": M{"type": "number", "integer": true},
				},
				"graphql": "mutation",
				"metrics": M{},
			},
		},
	}

	userOutput := M{
		"name":     M{"type": "string"},
		"lastname": M{"type": "string"},
		"age":      M{"type": "number", "integer": true},
	}

	//query example: user aggregate find action. return a paginated list of users.
	// can query for simple query+ query field or with complex queries using the query field.
	// - Query
	//  - user -> maps to user.get
	// 	- users -> maps to user.find
	// 	- expiredUsers -> maps to user.expiredUsers
	queryExample := M{
		"name": "user",
		"actions": map[string]M{
			// -> will map to query/user and  query/usersByIds for multiple ids
			"user.get": M{
				"name":    "user.get",
				"rawName": "get",
				"params": M{
					"ids":      M{"type": "array", "items": "string"},
					"fields":   M{"type": "array", "items": "string", "optional": true},
					"populate": M{"type": "array", "items": "string", "optional": true},
					"mapping":  M{"type": "boolean", "optional": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": M{},
			},
			// -> will map to query/users
			"user.list": M{
				"name":    "user.list",
				"rawName": "list",
				"params": M{
					"query":        M{"type": "object", "optional": true},
					"search":       M{"type": "string", "optional": true},
					"searchFields": M{"type": "array", "items": "string", "optional": true},
					"fields":       M{"type": "array", "items": "string", "optional": true},
					"populate":     M{"type": "array", "items": "string", "optional": true},
					"sort":         M{"type": "string", "optional": true},
					"page":         M{"type": "number", "integer": true, "optional": true},
					"pageSize":     M{"type": "number", "integer": true, "optional": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": M{},
			},
			//custom action query/expiredUsers
			"user.expiredUsers": M{
				"name":    "user.expiredUsers",
				"rawName": "expiredUsers",
				"params": M{
					"periodFrom": M{"type": "number", "integer": true},
					"periodTo":   M{"type": "number", "integer": true},
				},
				"output":  userOutput,
				"graphql": "query",
				"metrics": M{},
			},
		},
	}

	It("queryFields should create graphql fields based on action definitions ", func() {
		svc := GraphQLService{}
		p := payload.New(queryExample)
		fields := svc.queryActionsToFields(queryExample, p.Get("actions").RawMap())
		Expect(fields).ShouldNot(BeNil())
		Expect(fields["user"]).ShouldNot(BeNil())

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

	FIt("queryActions should return actions that are graphql query", func() {
		svc := GraphQLService{}
		queryExample["user.nonGrapQl"] = M{
			"name":    "user.nonGrapQl",
			"rawName": "nonGrapQl",
			"params": M{
				"ids": M{"type": "array", "items": "string"},
			},
			"metrics": M{},
		}
		actions := svc.queryActions(queryExample)
		Expect(len(actions)).Should(Equal(3))
	})
})
