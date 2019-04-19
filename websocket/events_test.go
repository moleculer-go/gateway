package websocket

import (
	"sync"

	"github.com/moleculer-go/moleculer"
	"github.com/moleculer-go/moleculer/context"
	"github.com/moleculer-go/moleculer/payload"
	"github.com/moleculer-go/moleculer/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("WebSockets Events", func() {

	It("onSubscribe should create a topic entry anda add the client", func() {
		delegates := test.DelegatesWithIdAndConfig(
			"onSubscribe_node",
			moleculer.Config{},
		)
		PublishCalled := false
		delegates.Publish = func(svcs ...interface{}) {
			PublishCalled = true
		}
		bkrContext := context.BrokerContext(delegates)
		client := &WebSocketClient{}

		em := EventsMixin{
			context:      bkrContext,
			clientTopics: &sync.Map{},
		}

		topic := "champ.champ"
		em.subscribeToMoleculerEvents(client, payload.New(map[string]interface{}{
			"topic": topic,
			"name":  "deviceToken",
			"value": "123oikjh",
		}))
		Expect(PublishCalled).Should(BeTrue())
		value, exists := em.clientTopics.Load(topic)
		Expect(exists).Should(BeTrue())
		Expect(value).ShouldNot(BeNil())
		te := value.(*TopicSubscription)
		Expect(te.topic).Should(Equal(topic))
		Expect(len(te.clients)).Should(Equal(1))
	})

	It("onSubscribe should reuse a topic entry and add a new client", func() {
		PublishCalled := 0
		delegates := test.DelegatesWithIdAndConfig(
			"onSubscribe_node",
			moleculer.Config{},
		)
		delegates.Publish = func(svcs ...interface{}) {
			PublishCalled++
		}
		bkrContext := context.BrokerContext(delegates)
		client := &WebSocketClient{}
		em := EventsMixin{
			context:      bkrContext,
			clientTopics: &sync.Map{},
		}

		topic := "champ.champ"
		em.subscribeToMoleculerEvents(client, payload.New(map[string]interface{}{
			"topic": topic,
			"name":  "deviceToken",
			"value": "123oikjh",
		}))
		Expect(PublishCalled).Should(Equal(1))
		value, exists := em.clientTopics.Load(topic)
		Expect(exists).Should(BeTrue())
		Expect(value).ShouldNot(BeNil())
		te := value.(*TopicSubscription)
		Expect(te.topic).Should(Equal(topic))
		Expect(len(te.clients)).Should(Equal(1))
		Expect(te.clients[0].name).Should(Equal("deviceToken"))
		Expect(te.clients[0].value).Should(Equal("123oikjh"))

		em.subscribeToMoleculerEvents(client, payload.New(map[string]interface{}{
			"topic": topic,
			"name":  "deviceToken",
			"value": "ssssss",
		}))
		Expect(PublishCalled).Should(Equal(1))
		value, exists = em.clientTopics.Load(topic)
		Expect(exists).Should(BeTrue())
		Expect(value).ShouldNot(BeNil())
		te = value.(*TopicSubscription)
		Expect(te.topic).Should(Equal(topic))
		Expect(len(te.clients)).Should(Equal(2))
		Expect(te.clients[0].name).Should(Equal("deviceToken"))
		Expect(te.clients[0].value).Should(Equal("ssssss"))

		em.subscribeToMoleculerEvents(client, payload.New(map[string]interface{}{
			"topic": topic + ".v2",
			"name":  "deviceToken",
			"value": "ssssss",
		}))
		Expect(PublishCalled).Should(Equal(2))
		value, exists = em.clientTopics.Load(topic + ".v2")
		Expect(exists).Should(BeTrue())
		Expect(value).ShouldNot(BeNil())
		te = value.(*TopicSubscription)
		Expect(te.topic).Should(Equal(topic + ".v2"))
		Expect(len(te.clients)).Should(Equal(1))
		Expect(te.clients[0].name).Should(Equal("deviceToken"))
		Expect(te.clients[0].value).Should(Equal("ssssss"))
	})

})
