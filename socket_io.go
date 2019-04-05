package gateway

import (
	"sync"

	"github.com/moleculer-go/moleculer"
	gosocketio "github.com/mtfelian/golang-socketio"
)

type clientEntry struct {
	//conn socketio.
}

type topicEntry struct {
	topic   string
	context moleculer.BrokerContext
	started bool
	list    []deliveryEntry
}

type deliveryEntry struct {
	name  string
	value string
}

// validate check if params should be added in the event stream
func (te *topicEntry) validate(params moleculer.Payload, name, value string) bool {
	pvalue := params.Get(name)
	if pvalue.Exists() && pvalue.String() == value {
		te.context.Logger().Debug("socker.io event validate() - record is valid !")
		return true
	}
	te.context.Logger().Debug("socker.io event validate() - record is invalid :(")
	return false
}

func eventService(topic string, handler moleculer.EventHandler) moleculer.Service {
	return moleculer.Service{
		Name: "socket_io-events_delivery-" + topic,
		Events: []moleculer.Event{
			{
				Name:    topic,
				Handler: handler,
			},
		},
	}
}

// startDelivery start the delivery of moleculer events to socket events.
func (te *topicEntry) startDelivery(ch *gosocketio.Channel, name, value string) {
	te.list = append(te.list, deliveryEntry{name, value})
	if te.started {
		te.context.Logger().Debug("startDelivery() topic already started! topic name: ", te.topic)
		return
	}
	te.started = true
	te.context.Logger().Debug("startDelivery() started for topic: ", te.topic)

	te.context.AddService(eventService(te.topic, func(context moleculer.Context, params moleculer.Payload) {
		context.Logger().Debug("event handler for topic: ", te.topic, " received params: ", params)
		for _, de := range te.list {
			if te.validate(params, de.name, de.value) {
				target := de.value + "." + te.topic
				content := string(jsonSerializer.PayloadToBytes(params))
				ch.Emit(target, content)
			}
		}
	}))
}

// createDelivery creates a delivery of moleculer events to a socket.io listener.
func createDelivery(context moleculer.BrokerContext, clients *sync.Map) func(ch *gosocketio.Channel, params interface{}) {
	handlers := &sync.Map{}
	return func(ch *gosocketio.Channel, data interface{}) {
		context.Logger().Debug("onEvent delivery -> data: ", data)
		var params moleculer.Payload
		bts, isBytes := data.([]byte)
		if isBytes {
			context.Logger().Debug("onEvent isBytes! ")
			params = jsonSerializer.BytesToPayload(&bts)
		}
		stng, isString := data.(string)
		if isString {
			context.Logger().Debug("onEvent isString! ")
			bts = []byte(stng)
			params = jsonSerializer.BytesToPayload(&bts)
		}
		topic := params.Get("topic").String()
		name := params.Get("name").String()
		value := params.Get("value").String()

		context.Logger().Debug("onEvent delivery -> topic: ", topic, " name: ", name, " value: ", value)
		temp, exists := handlers.Load(topic)
		var te topicEntry
		if exists {
			context.Logger().Debug("onEvent topicEntry found for target: ", topic)
			te = temp.(topicEntry)
		} else {
			te = topicEntry{context: context, topic: topic}
			handlers.Store(topic, te)
		}
		te.startDelivery(ch, name, value)
		target := value + "." + te.topic + ".setup"
		context.Logger().Debug("onEvent Develivery Started! -> target: ", target)
	}
}

// createIOServer Creates and start a new socket.io server.
func createIOServer(context moleculer.BrokerContext) *gosocketio.Server {
	server := gosocketio.NewServer()
	clients := &sync.Map{}

	if err := server.On(gosocketio.OnConnection, func(ch *gosocketio.Channel) {
		context.Logger().Debug("socketio connection conn.Id: ", ch.Id())
	}); err != nil {
		context.Logger().Error("createIOServer() server.On(connection) error: ", err)
	}

	if err := server.On("delivery", createDelivery(context, clients)); err != nil {
		context.Logger().Error("createIOServer() server.On(delivery) error: ", err)
	}

	if err := server.On(gosocketio.OnDisconnection, func(ch *gosocketio.Channel) {
		context.Logger().Debug("socketio disconnected ch.Id: ", ch.Id())
	}); err != nil {
		context.Logger().Error("createIOServer() server.On(OnDisconnection) error: ", err)
	}

	if err := server.On(gosocketio.OnError, func(ch *gosocketio.Channel, err interface{}) {
		context.Logger().Debug("socketio global error handler -> error: ", err)
	}); err != nil {
		context.Logger().Error("createIOServer() server.On(OnError) error: ", err)
	}
	return server
}
