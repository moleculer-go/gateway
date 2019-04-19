package gateway_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	gateway "github.com/moleculer-go/moleculer-web"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/broker"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/transit/memory"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

var logLevel = "info"

func createPrinterBroker(mem *memory.SharedMemory) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_printerBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})

	broker.Publish(moleculer.ServiceSchema{
		Name: "printer",
		Actions: []moleculer.Action{
			{
				Name: "print",
				Handler: func(context moleculer.Context, params moleculer.Payload) interface{} {
					context.Logger().Info("print action invoked. params: ", params)
					return fmt.Sprint("printed content: ", params.Get("content").String())
				},
			},
		},
		Events: []moleculer.Event{
			{
				Name: "printed",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					context.Logger().Info("printer.printed --> ", params.Value())
				},
			},
		},
	})

	return (*broker)
}

func createTempBroker(mem *memory.SharedMemory, prefix string) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_tempBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})
	broker.Publish(moleculer.ServiceSchema{
		Name: "temp",
		Started: func(ctx moleculer.BrokerContext, svc moleculer.ServiceSchema) {
			fmt.Println("temp service started -> prefix: ", prefix)
		},
		Actions: []moleculer.Action{
			{
				Name: "stuff",
				Handler: func(ctx moleculer.Context, params moleculer.Payload) interface{} {
					ctx.Logger().Info("I'm temp and I'm printing the content: ", params.Get("content").String())
					result := params.Get("content").String() + " " + prefix + "..."
					ctx.Emit("temp.stuffed", payload.Empty().Add("result", result))
					return result
				},
			},
		},
		Events: []moleculer.Event{
			{
				Name: "stuffed",
				Handler: func(context moleculer.Context, params moleculer.Payload) {
					context.Logger().Info("stuff.stuffed --> ", params.Value())
				},
			},
		},
	})
	return (*broker)
}

func createGatewayBroker(mem *memory.SharedMemory) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_gatewayBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})
	return (*broker)
}

var _ = Describe("API Gateway Integration Tests", func() {

	Describe("expose all actions and invoke via http", func() {

		It("should create a gateway with default settings", func() {
			mem := &memory.SharedMemory{}
			servicesBkr := createPrinterBroker(mem)
			gatewayBkr := createGatewayBroker(mem)

			gatewaySvc := &gateway.HttpService{Settings: map[string]interface{}{
				"port": "3552",
			}}
			gatewayBkr.Publish(gatewaySvc)
			servicesBkr.Start()
			gatewayBkr.Start()
			time.Sleep(300 * time.Millisecond)

			response, err := http.Get("http://localhost:3552/printer/print?content=HellowWorld")
			Expect(err).Should(BeNil())
			Expect(bodyContent(response)).Should(Equal("printed content: HellowWorld"))

			servicesBkr.Stop()
			gatewayBkr.Stop()
		})

		It("should discover new added service, reject call when service is removed, and accept again when service added", func() {
			mem := &memory.SharedMemory{}
			servicesBkr := createPrinterBroker(mem)
			gatewayBkr := createGatewayBroker(mem)

			port := "3553"
			host := "http://localhost:" + port
			gatewaySvc := &gateway.HttpService{Settings: map[string]interface{}{
				"port": port,
			}}
			gatewayBkr.Publish(gatewaySvc)
			servicesBkr.Start()
			gatewayBkr.Start()
			time.Sleep(300 * time.Millisecond)

			response, err := http.Get(host + "/printer/print?content=HellowWorld")
			Expect(err).Should(Succeed())
			Expect(bodyContent(response)).Should(Equal("printed content: HellowWorld"))

			tempBkr := createTempBroker(mem, "stuffed")
			tempBkr.Start()
			time.Sleep(400 * time.Millisecond)

			response, err = http.Get(host + "/temp/stuff?content=HellowWorld")
			Expect(err).Should(Succeed())
			Expect(bodyContent(response)).Should(Equal("HellowWorld stuffed..."))

			//remove the service
			tempBkr.Stop()
			time.Sleep(300 * time.Millisecond)
			response, err = http.Get(host + "/temp/stuff?content=HellowWorld")
			Expect(err).Should(Succeed())
			Expect(response.StatusCode).Should(Equal(500))
			Expect(bodyContent(response)).Should(Equal(`{"error":"Registry - endpoint not found for actionName: temp.stuff"}`))

			//start it again with modified service
			tempBkr = createTempBroker(mem, "reborn")
			tempBkr.Start()
			time.Sleep(500 * time.Millisecond)

			response, err = http.Get(host + "/temp/stuff?content=HellowAgain")
			Expect(err).Should(Succeed())
			Expect(bodyContent(response)).Should(Equal("HellowAgain reborn..."))

			tempBkr.Stop()
			servicesBkr.Stop()
			gatewayBkr.Stop()
		})

	})

})

// bodyContent return the reponse body as string
func bodyContent(resp *http.Response) string {
	defer resp.Body.Close()
	bts, err := ioutil.ReadAll(resp.Body)
	Expect(err).Should(Succeed())
	return string(bts)
}
