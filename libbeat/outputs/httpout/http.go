package httpout

import (
	"encoding/json"
	"net/http"
	"bytes"
	"fmt"
	"strconv"
	"io/ioutil"


	"github.com/elastic/beats/libbeat/common"
	"github.com/elastic/beats/libbeat/logp"
	"github.com/elastic/beats/libbeat/outputs"
)

var customHeaders = make(map[string]string)
var customFields  = make(map[string]string)
var capsuleObject map[interface{}]interface{}
var signature = ""
var currentEvent []common.MapStr
var count = 1;

func init() {
	outputs.RegisterOutputPlugin("http", HttpOutputPlugin{})
}


type HttpOutputPlugin struct {}

/*This is the outputer*/
type httpOutput struct {
	client http.Client
	url string
}

func initHttpObject(config *outputs.MothershipConfig) (*httpOutput, error){
	if config.Host == "" {
		return nil, fmt.Errorf("Failed to read host")
	}
	http_c := httpOutput{}
	http_c.client = http.Client{}

	var port int
	if config.Port == 0 {//Use default
		http_c.url = config.Host
	} else{
		port = config.Port
		http_c.url = config.Host + ":" + strconv.Itoa(port)
	}

	if config.CustomHeaders != nil { //Use headers from yaml file
		for k, v := range config.CustomHeaders {
            customHeaders[k] = v;
        } 
	}

	if config.CustomFields != nil { //Use fields from yaml file
		for k, v := range config.CustomFields {
            customFields[k] = v;
        } 
	}

	capsuleObject = config.Encapsulation
	if capsuleObject != nil {
		if config.EncapsulationSign == "" {
			return nil, fmt.Errorf("Failed to read message signature in http object")
		} else {
			signature = config.EncapsulationSign
		}
	}
	return &http_c, nil
}

func (h HttpOutputPlugin) NewOutput(
	config *outputs.MothershipConfig,
	topologyExpire int,
) (outputs.Outputer, error) {
	/*Create the http client*/
	output, err := initHttpObject(config)

	if err != nil {
		return nil, err
	}
	return output, nil
}

func substitueInterfaceArray(in []interface{}) []interface{} {
	result := make([]interface{}, len(in))
	for i, v := range in {
		result[i] = substitueMapValue(v)
	}
	return result
}


func substitueInterfaceMap(in map[interface{}]interface{}) common.MapStr {
	result := make(common.MapStr)
	for k, v := range in {
		result[fmt.Sprintf("%v", k)] = substitueMapValue(v)
	}
	return result
}

func substitueMapValue(v interface{}) interface{} {
	switch v := v.(type) {
	case []interface{}:
		return substitueInterfaceArray(v)
	case map[interface{}]interface{}:
		return substitueInterfaceMap(v)
	case string:
		if v == signature{
			return currentEvent
		}else{
			return v
		}
	default:
		return v
	}
}



func (out *httpOutput) PublishEvent(
	signaler outputs.Signaler,
	opts outputs.Options,
	event common.MapStr,
) error {
	for f,v := range customFields {
    	event[f] = v;
    }
    return out.PublishEvents(signaler, opts, []common.MapStr{event})
}

func (out *httpOutput) BulkPublish(
	signaler outputs.Signaler,
	opts outputs.Options,
	events []common.MapStr,
) error {
	for i := 0; i < len(events); i++ {
		for f,v := range customFields {
    		events[i][f] = v;
    	}
	}
	return out.PublishEvents(signaler, opts, events);
}



//The publish function
func (out *httpOutput) PublishEvents(
	signaler outputs.Signaler,
	opts outputs.Options,
	event []common.MapStr,
) error {
	var jsonEvent []byte
	var err error
	currentEvent = event
	if(signature != ""){
		ev := substitueInterfaceMap(capsuleObject)
		jsonEvent, err = json.Marshal(ev)
	}else {
		jsonEvent, err = json.Marshal(event)
	}
	
	if err != nil {
		logp.Err("Fail to convert the event to JSON: %s", err)
		outputs.SignalCompleted(signaler)
		return err
	}

	//Create http request
	req, err := http.NewRequest("POST", out.url, bytes.NewBuffer(jsonEvent))
	if err != nil {
		logp.Err("Fail to create http request: %s", err)
		outputs.SignalCompleted(signaler)
		return err
	}

	req.Header.Set("X-Custom-Header", "myvalue")
    req.Header.Set("Content-Type", "application/json")

    for h,v := range customHeaders {
    	req.Header.Set(h, v)
    }

    res, err := out.client.Do(req)
    if err != nil {
    	if opts.Guaranteed {
			logp.Critical("Unable to post http request: %s", err)
		} else {
			logp.Err("Error when post to http: %s", err)
		}
		outputs.SignalFailed(signaler, err)
		return err
    }

    logp.Debug("httpout", "Response status: %s", res.Status)

    body, _ := ioutil.ReadAll(res.Body)

    logp.Debug("httpout", "Response body: %s", string(body))
    defer res.Body.Close()

	outputs.SignalCompleted(signaler)
	return nil
}

