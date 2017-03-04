package main

import (
	"encoding/json"
	"time"
	"io/ioutil"
	"log"
	"net/http"
	"net"
	"os/exec"
	"strconv"
	"fmt"
	"os"
	"strings"
)

// API

var templow float64
var temphi  float64
var tempcur float64

var heating int

type Thermostat struct {
	TempLow   float64    `json:"templow"`
	TemoHi    float64    `json:"temphi"`
	TemoCur   float64    `json:"tempcur"`
}

type Settings struct {
	Thermostat
	RelayHost		string
	ListenOn		string
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

func setHeating(val int){
	conn, err := net.Dial("tcp", settings.RelayHost)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Fprintf(conn, "%d\r\n", val)

	conn.Close()
}

func controlLoop(){
	for true {
		t,err := getCur()
		if err == nil {
			tempcur = t
		}

		if tempcur > temphi {
			heating = 0
		}

		if tempcur < templow {
			heating = 1
		}

		go setHeating(heating)

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

	templow = t.TempLow
	temphi = t.TemoHi

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

	response := Thermostat{templow, temphi, tempcur}
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

    temphi = settings.TemoHi
    templow = settings.TempLow
    settingsFile.Close()
    return nil
}

func writeSettingsFile() error {
    settingsFile, err := os.Create("settings.json")
    if err != nil {
            return err
    }
    defer settingsFile.Close()

    settings.TempLow = templow
    settings.TemoHi = temphi

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

    tempcur = 99

	go controlLoop()

	// API
	http.HandleFunc("/api", apiHandler)
	http.Handle("/", http.FileServer(http.Dir("./ui")))
	http.ListenAndServe(settings.ListenOn, nil)

}
