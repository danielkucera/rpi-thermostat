package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// API

type Thermostat struct {
	TempLow   float64   `json:"templow"`
	TempDiff  float64   `json:"tempdiff"`
	TempCur   float64   `json:"tempcur"`
	UpdatedAt time.Time `json:"updatedat"`
	HeatingOn int       `json:"heatingon"`
}

type Settings struct {
	Thermostat
	RelayHost string
	ListenOn  string
}

var settings Settings

func getCur() (float64, error) {
	out, err := exec.Command("/usr/local/bin/temper-poll", "-c").Output()
	if err != nil {
		log.Println(err)
		return 0, err
	}

	f, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		log.Println(err)
		return 0, err
	}

	return f, nil
}

func setHeating(val int) {
	conn, err := net.Dial("tcp", settings.RelayHost)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(conn, "%d\r\n", val)

	conn.Close()
}

func controlLoop() {
	for true {
		t, err := getCur()
		if err == nil {
			settings.TempCur = t
			settings.UpdatedAt = time.Now()
		}

		if settings.TempCur > settings.TempLow + settings.TempDiff {
			settings.HeatingOn = 0
		}

		if settings.TempCur < settings.TempLow {
			settings.HeatingOn = 1
		}

		go setHeating(settings.HeatingOn)

		time.Sleep(10 * time.Second)
	}
}

func updateHandler(r *http.Request) int {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return http.StatusBadRequest
	}

	var t Thermostat
	err = json.Unmarshal(body, &t)
	if err != nil {
		return http.StatusBadRequest
	}

	settings.TempLow = t.TempLow
	settings.TempDiff = t.TempDiff

	writeSettingsFile()

	return http.StatusOK
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	var code int

	switch r.Method {
	case "GET":
		code = http.StatusOK
	case "POST":
		code = updateHandler(r)
	default:
		code = http.StatusNotImplemented
	}

	response := settings
	json, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "Error marshalling JSON", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if code != 200 {
		http.Error(w, "", code)
	}
	w.Write(json)
	log.Printf("%s %s %s %d", r.RemoteAddr, r.Method, r.URL, 200)
}

func readSettingsFile() error {
	settingsFile, err := os.Open("settings.json")
	if err != nil {
		return fmt.Errorf("settings.json file not found, using defaults")
	}

	if err = json.NewDecoder(settingsFile).Decode(&settings); err != nil {
		return err
	}

	settingsFile.Close()
	return nil
}

func writeSettingsFile() error {
	settingsFile, err := os.Create("settings.json")
	if err != nil {
		return err
	}
	defer settingsFile.Close()

	output, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = settingsFile.Write(output)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	err := readSettingsFile()
	if err != nil {
		log.Printf("WARNING: %s", err)
	}
	defer writeSettingsFile()

	settings.TempCur = 99

	go controlLoop()

	// API
	http.HandleFunc("/api", apiHandler)
	http.Handle("/", http.FileServer(http.Dir("./ui")))
	http.ListenAndServe(settings.ListenOn, nil)

}
