package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fleetdm/fleet/v4/pkg/fleethttp"
)

const DELAY = 10 * time.Second // adjust this to simulate slow webhook response

var (
	apiTokenFlag = flag.String("api-token", "", "API token")
	deviceIDFlag = flag.String("device-id", "", "Device ID to unenroll")
)

type simpleClient struct {
	apiToken string
}

func newSimpleClient(apiToken string) *simpleClient {
	return &simpleClient{
		apiToken: apiToken,
	}
}

// unenroll sends a request to the SimpleMDM API unenroll a device by its ID, see
// https://api.simplemdm.com/v1#unenroll
func (c *simpleClient) unenroll(deviceID uint) error {
	client := fleethttp.NewClient()
	path := fmt.Sprintf("https://a.simplemdm.com/api/v1/devices/%d/unenroll", deviceID)
	req, err := http.NewRequest("POST", path, nil)
	if err != nil {
		return err
	}
	req.SetBasicAuth(*apiTokenFlag, "")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyText, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Printf("Unenrolling device %d\n%s", deviceID, bodyText)

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status code %d while unenrolling device %d", resp.StatusCode, deviceID)
	}

	return nil
}

func main() {
	flag.Parse()

	if *apiTokenFlag == "" {
		log.Fatal("--api-token must be provided")
	}

	if *deviceIDFlag == "" {
		log.Fatal("--device-id must be provided")
	}
	deviceID, err := strconv.ParseUint(*deviceIDFlag, 10, 32)
	if err != nil {
		log.Fatalf("invalid device ID %s: %v", *deviceIDFlag, err)
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		var detail string
		body, err := io.ReadAll(request.Body)
		if err != nil {
			detail = fmt.Sprintf("| ERROR: reading request body: %s", err)
		} else if len(body) != 0 {
			detail = fmt.Sprintf("| BODY: %s", string(body))
		}
		log.Printf("%s %s %s\n", request.Method, request.URL.Path, detail)

		// NOTE: this webhook handler currently only supports unenrolling the single
		// device ID provided via the --device-id command line flag, regardless of
		// which device's webhook fired. See
		// https://github.com/fleetdm/fleet/issues (tracked separately) for the
		// follow-up work needed to parse the webhook payload's device serial number
		// and look up the corresponding device ID via the SimpleMDM API
		// (https://api.simplemdm.com/v1#list-all-6) before unenrolling.

		time.Sleep(DELAY)

		if err := newSimpleClient(*apiTokenFlag).unenroll(uint(deviceID)); err != nil {
			log.Printf("error unenrolling device %d: %s", deviceID, err.Error())
			writer.WriteHeader(http.StatusBadGateway)
			if _, err := writer.Write([]byte("Error unenrolling device")); err != nil {
				log.Printf("error writing response %s", err.Error())
			}
			return
		}

		if _, err := writer.Write(nil); err != nil {
			log.Printf("error writing response %s", err.Error())
		}
	})

	port := ":4648"
	if p := os.Getenv("SERVER_PORT"); p != "" {
		port = p
	}

	fmt.Printf("Server running at http://localhost%s\n", port)
	server := &http.Server{
		Addr:              port,
		ReadHeaderTimeout: 30 * time.Second,
	}
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("server error %s", err.Error())
		}
	}()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig // block on signal
}
