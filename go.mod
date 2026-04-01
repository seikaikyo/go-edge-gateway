module github.com/seikaikyo/go-edge-gateway

go 1.26.1

require (
	github.com/dashfactory/go-factory-io v1.0.0
	github.com/eclipse/paho.mqtt.golang v1.5.1
	github.com/goburrow/modbus v0.1.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/goburrow/serial v0.1.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
)

replace github.com/dashfactory/go-factory-io => ../go-factory-io
