package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/fleetdm/fleet/v4/pkg/fleethttp"
)

var (
	username = flag.String("username", "", "JSS username")
	password = flag.String("password", "", "JSS password")
	url      = flag.String("url", "", "URL of the Jamf instance")
	port     = flag.String("port", "4648", "Port used by the webserver")
)

func main() {
	flag.Parse()

	if *username == "" || *password == "" || *url == "" {
		log.Fatal("--username, --password, and --url are required.")
	}

	log.Println("getting API token...")
	client, err := newJamfClient(*username, *password, *url)
	if err != nil {
		log.Fatalf("initializing Jamf client: %s", err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(io.LimitReader(request.Body, 1<<20))
		if err != nil {
			log.Printf("ERROR: reading request body: %s\n", err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}

		if len(body) == 0 {
			log.Print("ERROR: empty request body")
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		var deviceInfo struct {
			Host struct {
				HardwareSerial string `json:"hardware_serial"`
			} `json:"host"`
		}
		if err := json.Unmarshal(body, &deviceInfo); err != nil {
			log.Printf("ERROR: unmarshalling request body: %s\n", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("unenrolling %s", deviceInfo.Host.HardwareSerial)

		jamfID, err := client.getJamfID(deviceInfo.Host.HardwareSerial)
		if err != nil {
			log.Printf("ERROR: getting Jamf ID from serial number: %s\n", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Println("attempting to remove device from Jamf management...")
		if err := client.unmanageDevice(jamfID); err != nil {
			log.Printf("ERROR: unmanaging device: %s\n", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}

		log.Printf("%s unenrolled", deviceInfo.Host.HardwareSerial)
	})

	log.Printf("server running at http://localhost:%s\n", *port)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%s", *port),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err.Error())
	}
}

type jamfClient struct {
	url   string
	token string
}

func newJamfClient(username, password, url string) (*jamfClient, error) {
	client := &jamfClient{url: url}
	var err error
	if client.token, err = client.getBearerToken(username, password); err != nil {
		return nil, fmt.Errorf("getting bearer token: %w", err)
	}
	return client, nil
}

func (j *jamfClient) doWithRequest(req *http.Request) ([]byte, error) {
	client := fleethttp.NewClient()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode > 299 {
		return body, fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	return body, nil
}

func (j *jamfClient) do(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Add("accept", "application/xml")
	req.Header.Add("Authorization", "Bearer "+j.token)
	body, err := j.doWithRequest(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	return body, nil
}

func (j *jamfClient) unmanageDevice(jamfID string) error {
	_, err := j.do("POST", fmt.Sprintf("%s/JSSResource/computercommands/command/UnmanageDevice/id/%s", *url, jamfID))
	if err != nil {
		return fmt.Errorf("unmanaging device %s: %w", jamfID, err)
	}
	return nil
}

func (j *jamfClient) getBearerToken(username, password string) (string, error) {
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/auth/token", *url), nil)
	if err != nil {
		return "", fmt.Errorf("creating auth token request: %w", err)
	}
	req.SetBasicAuth(username, password)

	body, err := j.doWithRequest(req)
	if err != nil {
		return "", fmt.Errorf("requesting bearer token: %w", err)
	}

	var tokenResponse struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", fmt.Errorf("unmarshalling token response: %w", err)
	}

	return tokenResponse.Token, nil
}

func (j *jamfClient) getJamfID(serial string) (string, error) {
	body, err := j.do("GET", fmt.Sprintf("%s/JSSResource/computers/serialnumber/%s", *url, serial))
	if err != nil {
		return "", fmt.Errorf("getting computer by serial number %s: %w", serial, err)
	}

	var data struct {
		XMLName xml.Name `xml:"computer"`
		ID      string   `xml:"general>id"`
	}

	if err := xml.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("unmarshalling computer XML response: %w", err)
	}

	return data.ID, nil
}
