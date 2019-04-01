package gateway_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/moleculer-go/moleculer"
	gateway "github.com/moleculer-go/moleculer-web"
	"github.com/moleculer-go/moleculer/broker"
	"github.com/moleculer-go/moleculer/transit/memory"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

var logLevel = "Error"

func createPrinterBroker(mem *memory.SharedMemory) broker.ServiceBroker {
	broker := broker.New(&moleculer.Config{
		DiscoverNodeID: func() string { return "node_printerBroker" },
		LogLevel:       logLevel,
		TransporterFactory: func() interface{} {
			transport := memory.Create(log.WithField("transport", "memory"), mem)
			return &transport
		},
	})

	broker.AddService(moleculer.Service{
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
		mem := &memory.SharedMemory{}
		servicesBkr := createPrinterBroker(mem)
		gatewayBkr := createGatewayBroker(mem)

		BeforeSuite(func() {
			servicesBkr.Start()
			gatewayBkr.Start()
		})

		AfterSuite(func() {
			servicesBkr.Stop()
			gatewayBkr.Stop()
		})

		It("should create a gateway with default settings", func() {
			gatewaySvc := gateway.Service()
			gatewayBkr.AddService(gatewaySvc)
			time.Sleep(300 * time.Millisecond)

			value := func(param string) string {
				return ""
			}
			fmt.Println("type for func() --> ", fmt.Sprintf("%T", value))

			response, err := http.Get("http://localhost:3100/printer/print?content=HellowWorld")
			Expect(err).Should(BeNil())

			defer response.Body.Close()
			bodyBytes, err := ioutil.ReadAll(response.Body)
			sBody := string(bodyBytes)
			Expect(sBody).Should(Equal("printed content: HellowWorld"))
		})

	})

})
