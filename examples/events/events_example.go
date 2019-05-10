package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/moleculer-go/moleculer"

	gateway "github.com/moleculer-go/gateway"
	"github.com/moleculer-go/moleculer/broker"
)

type TruckService struct {
}

func (svc *TruckService) Name() string {
	return "truck"
}

func (svc *TruckService) Drive(ctx moleculer.Context) {
	ctx.Emit("truck.drive.started", time.Now())
}

func main() {
	truckSvc := &TruckService{}
	gatewaySvc := &gateway.HttpService{
		Settings: map[string]interface{}{"port": "9015"},
		Depends:  []string{"truck"},
	}

	bkr := broker.New(&moleculer.Config{LogLevel: "error"})
	bkr.Publish(gatewaySvc, truckSvc)
	bkr.Start()

	cmd := exec.Command("node", "client.js")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		log.Printf("Command finished with error: %v", err)
	}
	bkr.Stop()
}

// bodyContent return the response body as string
func bodyContent(resp *http.Response) string {
	defer resp.Body.Close()
	bts, _ := ioutil.ReadAll(resp.Body)
	return string(bts)
}
