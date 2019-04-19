package websocket

import (
	"sync"

	"github.com/moleculer-go/moleculer"
)

type EventsMixin struct {
	context      moleculer.BrokerContext
	clientTopics *sync.Map
}

func (m *EventsMixin) Init(context moleculer.BrokerContext, s *WebSocketServer) {
	m.context = context
	s.sub("moleculer.events", m.subscribeToMoleculerEvents)
}

func (m *EventsMixin) ensureClientTopics() {
	if m.clientTopics == nil {
		m.clientTopics = &sync.Map{}
	}
}

// getOrCreateTopic if the topic exists return the TopicSubscription,
// is not creates one, store in it and return it.
func (m *EventsMixin) getOrCreateTopic(topic string) *TopicSubscription {
	m.ensureClientTopics()
	temp, exists := m.clientTopics.Load(topic)
	if exists {
		m.context.Logger().Debug("getTopicSubscription TopicSubscription found for topic: ", topic)
		te := temp.(*TopicSubscription)
		return te
	}
	te := TopicSubscription{context: m.context, topic: topic}
	m.clientTopics.Store(topic, &te)
	temp, _ = m.clientTopics.Load(topic)
	return temp.(*TopicSubscription)
}

// subscribeToMoleculerEvents is called when a client subscribes
// to receive moleculer events on a websocket connection.
func (m *EventsMixin) subscribeToMoleculerEvents(client *WebSocketClient, params moleculer.Payload) {
	logger := m.context.Logger()
	logger.Debug("subscribeToMoleculerEvents params: ", params)

	topic := params.Get("topic").String()
	client.name = params.Get("name").String()
	client.value = params.Get("value").String()

	te := m.getOrCreateTopic(topic)
	te.addClient(client)

	target := client.value + "." + te.topic + ".setup"
	logger.Debug("subscribeToMoleculerEvents Delivery Started! -> target: ", target)
}

type TopicSubscription struct {
	topic   string
	context moleculer.BrokerContext
	started bool
	clients []*WebSocketClient
}

// validate check if params should be added in the event stream.
func (te *TopicSubscription) validate(params moleculer.Payload, client *WebSocketClient) bool {
	pvalue := params.Get(client.name)
	if pvalue.Exists() && pvalue.String() == client.value {
		te.context.Logger().Debug("TopicSubscription validate() - record is valid !")
		return true
	}
	te.context.Logger().Debug("TopicSubscription validate() - record is invalid :(")
	return false
}

// websocketFunnelService create a service schema for events funnel handler.
// which will listen for the events -> topic and call the handler.
func websocketFunnelService(topic string, handler moleculer.EventHandler) moleculer.ServiceSchema {
	return moleculer.ServiceSchema{
		Name: "websocket-events_funnel-" + topic,
		Events: []moleculer.Event{
			{
				Name:    topic,
				Handler: handler,
			},
		},
	}
}

func (te *TopicSubscription) stop() {
	if !te.started {
		te.context.Logger().Debug("stop() topic not started! -> ", te.topic)
		return
	}
	te.context.Logger().Debug("stop() stoping topic -> ", te.topic)

}

// start create a moleculer service to listen for the events on the topic.
// when event occours deliver to the client.
func (te *TopicSubscription) start() {
	if te.started {
		te.context.Logger().Debug("start() topic already started! -> ", te.topic)
		return
	}
	te.started = true
	te.context.Logger().Debug("start() for topic: ", te.topic)

	te.context.Publish(websocketFunnelService(te.topic, func(context moleculer.Context, params moleculer.Payload) {
		context.Logger().Debug("event handler for topic: ", te.topic, " received params: ", params)
		for _, client := range te.clients {
			if te.validate(params, client) {
				target := client.value + "." + te.topic
				client.pub(target, params)
			}
		}
	}))
}

// addClient start the delivery of moleculer events to socket events.
func (te *TopicSubscription) addClient(client *WebSocketClient) {
	te.clients = append(te.clients, client)
	//when client close.. remove from the list of clients
	//inside the topic entry
	client.onClose = func() {
		if client.closed {
			return
		}
		client.closed = true
		te.cleanUpClients(client.id)
	}
	te.start()
}

func (te *TopicSubscription) cleanUpClients(clientId string) {
	list := make([]*WebSocketClient, len(te.clients)-1)
	for _, item := range te.clients {
		if item.id != clientId {
			list = append(list, item)
		}
	}
	te.clients = list
}
