package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type HomePage struct {
	Response string
}

type ClientData struct {
	VideoUrl    string
	AccessToken string
}

// Homepage Route
func homePageHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)

	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// if (*r).Method == "OPTIONS" {
	// 	return
	// }

	var data HomePage
	data.Response = "Hello World!"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func likeSongHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)

	reqBody, _ := ioutil.ReadAll(r.Body)
	var data ClientData
	json.Unmarshal(reqBody, &data)

	odesli(&data)

}

// Get song data from Odesli api
func odesli(data *ClientData) {
	query := strings.Split(data.VideoUrl, "&")[0]
	odesliApiKey := os.Getenv("ODESLI_API_KEY")
	params := "https://api.song.link/v1-alpha.1/links?" +
		"url=" + url.QueryEscape(query) + "&" +
		"platform=youtube" + "&" +
		"key=" + url.QueryEscape(odesliApiKey)

	res, _ := http.Get(params)
	defer res.Body.Close()

	body, _ := ioutil.ReadAll(res.Body)
	var tempData map[string]interface{}
	err := json.Unmarshal([]byte(body), &tempData)
	if err != nil {
		panic(err)
	}

	if (tempData["linksByPlatform"].(map[string]interface{}))["spotify"] != nil {
		uniqueId := ((tempData["linksByPlatform"].(map[string]interface{}))["spotify"].(map[string]interface{}))["entityUniqueId"]
		fmt.Println(uniqueId)
		// spotifyData := tempData["entityUniqueId"].(map[string]interface{})
	}

}

// Request handler
func handleRequests(mux *http.ServeMux) {
	mux.HandleFunc("/", homePageHandler)
	mux.HandleFunc("/api/like_song", likeSongHandler)
	log.Fatal(http.ListenAndServe(":3000", mux))
}

// Set up response headers
func setupResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

// ENV setup
// func getEnv(key string) {
// 	err := godotenv.Load(".env")
// 	if err != nil {
// 		log.Fatal("Error loading .env file")
// 	}
// }

func main() {
	// Load env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Run the server
	mux := http.NewServeMux()
	fmt.Println("Server running at localhost:3000")
	handleRequests(mux)
}
