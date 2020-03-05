package elast

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v7"
	"github.com/elastic/go-elasticsearch/v7/esapi"
	"github.com/jonboulle/clockwork"
)


//Kairos Drive ... product
const Kairos string = "Kairos Drive"

//KairosService Kairos Drive OS Service
const KairosService string = "Kairos Drive OS Service"

//LogMessage ... used for logging feature
type LogMessage struct {
	Code      string
	Message   string
	Parameter string
	Product   string
	Component string
	Data      Params
}

//Params array of logging parameters
type Params []parameter

type parameter struct {
	Param string
	Value string
}

//Add adds param to list
func (p Params) Add(prop string, val string) Params {
	p = append(p, parameter{Param: prop, Value: val})
	return p
}

//Log ... generate log message
func Log(r *http.Request, message LogMessage) {

	envElastTimeout, ok := os.LookupEnv("elast_timeout")
	elastTimeout := 1
	if !ok {
		log.Println("elast_timeout environment variable required but not set. Using default 1")
	} else {
		elastTimeout, _ = strconv.Atoi(envElastTimeout)
	}

	es, elastEnable, err := Config(elastTimeout)

	if err != nil {
		elastEnable = false
	}
	str := "0"
	usrIP := ""

	if r != nil {
		usrid := r.Context().Value("userID")
		str = fmt.Sprintf("%v", usrid)
		usrIP = r.Header.Get("X-Forwarded-For")
	}

	if str == "<nil>" {
		str = "0"
	}

	if elastEnable == true {

		i := time.Now().Nanosecond()

		logtime := elasticCurrentTimeFormat()
		// Set up the request object.
		req := esapi.IndexRequest{
			Index:      "application",
			DocumentID: strconv.FormatInt(int64(i), 20),
			Body: strings.NewReader(`
		{
			"@timestamp": "` + logtime + `",
			"Time": "` + logtime + `",
			"Code": "` + message.Code + `",
			"UserID": "` + str + `",
			"Message": "` + message.Message + `",
			"Parameter": "` + message.Parameter + `",
			"Product": "` + message.Product + `",
			"Component": "` + message.Component + `",
			"RemoteIP": "` + GetLocalIP() + `",
			"ClientIP": "` + usrIP + `",
			"Data": ` + generateData(message.Data) + `
		}`),
			Refresh: "true",
		}

		res, err := req.Do(context.Background(), es)
		defer res.Body.Close()
		if err != nil {
			log.Println("Error getting response while sending log to elastic")
		} else {

			if res.IsError() {
				log.Printf("[%s] Error indexing document ID=%d"+res.String(), res.Status(), i)
			} else {
				// Deserialize the response into a map.
				var r map[string]interface{}
				if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
					log.Printf("Error parsing the response body: %s", err)
				} else {
					// Print the response status and indexed document version.
					log.Printf("[%s] %s; version=%d with timestamp: "+logtime, res.Status(), r["result"], int(r["_version"].(float64)))
				}
			}
		}
	}
	return
}

//LogEasy short version of Log method
func LogEasy(r *http.Request, uCode string, uMessage string, uParam Params, uComponent string) {
	m := LogMessage{Code: uCode, Message: uMessage, Data: uParam, Product: Kairos, Component: uComponent}
	Log(r, m)
}

//LogSys short version of Log method for service errors
func LogSys(r *http.Request, uCode string, uMessage string, uParam Params, uComponent string) {
	m := LogMessage{Code: uCode, Message: uMessage, Data: uParam, Product: KairosService, Component: uComponent}
	Log(r, m)
}

//Config ... checks and sets environment params
func Config(srvTimeout int) (es *elasticsearch.Client, elastEnable bool, err error) {

	reqTimeout := time.Duration(srvTimeout) * time.Second

	elastEnable = true

	tstate, ok := os.LookupEnv("elast_enable")
	if !ok {
		log.Println("elast_enable environment variable required but not set. Using default true => elasticsearch enabled")
	} else {
		elastEnable, _ = strconv.ParseBool(tstate)
	}

	elastHost, ok := os.LookupEnv("elast_host")
	if !ok {
		log.Println("elast_host environment variable required but not set. Elasticsearch feature will be disabled")
		elastEnable = false
		return
	}
	elastPort, ok := os.LookupEnv("elast_port")
	if !ok {
		log.Println("elast_port environment variable required but not provided. Elasticsearch feature will be disabled")
		elastEnable = false
		return
	}

	cfg := elasticsearch.Config{
		Addresses: []string{
			elastHost + ":" + elastPort,
		},
		Transport: &http.Transport{
			MaxIdleConnsPerHost:   10,
			ResponseHeaderTimeout: reqTimeout,
			DialContext:           (&net.Dialer{Timeout: reqTimeout}).DialContext,
		},
	}
	es, err = elasticsearch.NewClient(cfg)
	if err != nil {
		return
	}
	res, err := es.Cluster.Health()
	if err != nil {
		return
	}
	defer res.Body.Close()
	return
}

//GetLocalIP ... read local ip adress
func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

//clocks as arg array to make it optional. but only first argument is used to generate time string
func elasticCurrentTimeFormat(clocks ...time.Time) string {
	var clock time.Time
	if len(clocks) == 0 {
		clock = clockwork.NewRealClock().Now()
	} else {
		clock = clocks[0]
	}
	return clock.Format("2006-01-02T15:04:05.000000-0700")
}

func serializeParams(pms Params) (seriString string) {
	seriString = ""
	for _, sparam := range pms {
		seriString = seriString + sparam.Param + ": " + sparam.Value + ", "
	}
	return
}

func generateData(pms Params) (jsonString string) {
	jsonString = ""
	if pms != nil {
		for _, sparam := range pms {
			jsonString += `
			"` + sparam.Param + `": "` + sparam.Value + `",`
		}
		if last := len(jsonString) - 1; last >= 0 && jsonString[last] == ',' {
			jsonString = jsonString[:last]
		}
	}
	jsonString = "{" + jsonString + "}"
	return
}
