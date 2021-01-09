package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/vitaly-kashtalyan/hlk-sw16"
	"net/http"
	"os"
	"time"
)

const (
	ENABLE           = "on"
	DISABLE          = "off"
	HlkSw16Host      = "HLK_SW16_HOST"
	HlkSw16Port      = "HLK_SW16_PORT"
	MqttSenderHost   = "MQTT_SENDER_HOST"
	Table            = "relay"
	HomeSensorsRelay = "home/sensors/relay"
)

type Relays struct {
	Relay []Relay `json:"relays"`
}

type Relay struct {
	Id    int `json:"id"`
	State int `json:"state"`
}

type BaseResponse struct {
	Message string `json:"message"`
}

type Switcher struct {
	ID     int    `json:"id" binding:"required"`
	Switch string `json:"switch" binding:"required"`
}

type Message struct {
	Topic    string `json:"topic"`
	Qos      int    `json:"qos"`
	Retained bool   `json:"retained"`
	Payload  string `json:"payload"`
}

func main() {
	// Echo instance
	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// Routes
	e.GET("/health", health)
	e.GET("/status", getStatus)
	e.POST("/relay", switcher)

	// Start server
	e.Logger.Fatal(e.Start(":8080"))
}

func health(c echo.Context) error {
	return c.JSON(http.StatusOK, BaseResponse{
		Message: http.StatusText(http.StatusOK),
	})
}

func switcher(c echo.Context) error {
	var jsonBody Switcher
	var errMsg error

	err := c.Bind(&jsonBody)
	if err != nil {
		return resError(c, http.StatusBadRequest, err)
	}

	err = validateSwitcher(jsonBody)
	if err != nil {
		return resError(c, http.StatusBadRequest, err)
	}

	hlk := getConnect()
	if hlk.Err != nil {
		return resError(c, http.StatusServiceUnavailable, hlk.Err)
	}

	if jsonBody.Switch == ENABLE {
		err = hlk.RelayOn(jsonBody.ID)
		errMsg = sendMessage(getMessage(HomeSensorsRelay, fmt.Sprintf("%s,id=%d value=true", Table, jsonBody.ID)))
	} else if jsonBody.Switch == DISABLE {
		err = hlk.RelayOff(jsonBody.ID)
		errMsg = sendMessage(getMessage(HomeSensorsRelay, fmt.Sprintf("%s,id=%d value=false", Table, jsonBody.ID)))
	}
	if errMsg != nil {
		return resError(c, http.StatusInternalServerError, errMsg)
	}
	if err != nil {
		return resError(c, http.StatusBadRequest, err)
	}

	msg, err := hlk.ReadMessage()
	if err != nil {
		return resError(c, http.StatusInternalServerError, err)
	}

	if err = hlk.Close(); err != nil {
		return resError(c, http.StatusInternalServerError, err)
	}

	return c.JSON(http.StatusOK, setMapRelays(msg))
}

func validateSwitcher(switcher Switcher) error {
	if switcher.Switch != ENABLE && switcher.Switch != DISABLE {
		j, _ := json.Marshal(switcher)
		return fmt.Errorf("switch must be: '" + ENABLE + "' or '" + DISABLE + "'; body:" + string(j))
	}
	return nil
}

func getStatus(c echo.Context) error {
	for i := 0; i < 10; i++ {
		hlk := getConnect()
		if hlk.Err != nil {
			return resError(c, http.StatusServiceUnavailable, hlk.Err)
		}

		if err := hlk.StatusRelays(); err != nil {
			return resError(c, http.StatusBadRequest, err)
		}

		msg, err := hlk.ReadMessage()
		if err != nil {
			return resError(c, http.StatusBadRequest, err)
		}

		if validateAnswer(msg) {
			return c.JSON(http.StatusOK, setMapRelays(msg))
		} else {
			time.Sleep(1000)
		}
		if err = hlk.Close(); err != nil {
			return resError(c, http.StatusInternalServerError, err)
		}
	}
	return resError(c, http.StatusInternalServerError, fmt.Errorf("unexpected error"))
}

func getConnect() (c *hlk_sw16.Connection) {
	return hlk_sw16.New(getHlkSw16Host(), getHlkSw16Port())
}

func setMapRelays(msg []byte) (relays Relays) {
	var relay []Relay
	for index, element := range msg {
		if index > 1 && index < 18 {
			status := int(element)
			if status == 2 {
				status = 0
			}
			relay = append(relay, Relay{
				Id:    index - 2,
				State: status,
			})
		}
	}
	return Relays{Relay: relay}
}

func validateAnswer(msg []byte) bool {
	for index, element := range msg {
		if index > 1 && index < 18 {
			if int(element) > 2 {
				return false
			}
		}
	}
	return true
}

func resError(c echo.Context, statusCode int, err error) error {
	return c.JSON(statusCode, BaseResponse{
		Message: err.Error(),
	})
}

func sendMessage(message Message) error {
	uri := fmt.Sprintf("http://%s/publish", getMqttSenderHost())
	body := new(bytes.Buffer)
	err := json.NewEncoder(body).Encode(message)
	if err != nil {
		return fmt.Errorf("%q: %v", uri, err)
	}
	resp, err := http.Post(uri, "application/json; charset=utf-8", body)
	if err != nil {
		return fmt.Errorf("cannot fetch URL %q: %v", uri, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected http POST status: %s", resp.Status)
	}
	return nil
}

func getMessage(topic string, payload string) Message {
	return Message{Topic: topic, Qos: 2, Retained: false, Payload: payload}
}

func getHlkSw16Host() string {
	if len(os.Getenv(HlkSw16Host)) == 0 {
		//_ = os.Setenv(HlkSw16Host, "192.168.16.254")
		_ = os.Setenv(HlkSw16Host, "192.168.0.200")
	}
	return os.Getenv(HlkSw16Host)
}

func getHlkSw16Port() string {
	if len(os.Getenv(HlkSw16Port)) == 0 {
		_ = os.Setenv(HlkSw16Port, "8080")
	}
	return os.Getenv(HlkSw16Port)
}

func getMqttSenderHost() string {
	//return os.Getenv(MqttSenderHost)
	return "localhost:80755"
}
