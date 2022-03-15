package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
)

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

	w.Write([]byte("<h1>Hello World!</h1>"))
}

func likeSongHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)

	reqBody, _ := ioutil.ReadAll(r.Body)
	var data ClientData
	json.Unmarshal(reqBody, &data)

	fmt.Println(data.AccessToken)
	fmt.Println(data.VideoUrl)

}

// Request handler
func handleRequests(mux *http.ServeMux) {
	mux.HandleFunc("/", homePageHandler)
	mux.HandleFunc("/like_song", likeSongHandler)
	log.Fatal(http.ListenAndServe(":3000", mux))
}

func setupResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

func main() {
	// Run the server
	mux := http.NewServeMux()
	fmt.Println("Server running at localhost:3000")
	handleRequests(mux)
}
