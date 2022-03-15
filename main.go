package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/AudDMusic/audd-go"
	"github.com/joho/godotenv"
	"github.com/kkdai/youtube/v2"
	fluentffmpeg "github.com/modfy/fluent-ffmpeg"
)

// Homepage json
type HomePage struct {
	Response string
}

// Store data being posted by Chrome extension
type ClientData struct {
	VideoUrl    string
	AccessToken string
}

// Store identified song information
type SongData struct {
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	SpotifyId string `json:"spotifyId"`
}

// Used to create and return error json if no data is found
type Error struct {
	Error string `json:"error"`
}

// Homepage Route
func homePageHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)

	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var data HomePage
	data.Response = "Hello World!"

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// Handle Chrome extension api request
func likeSongHandler(w http.ResponseWriter, r *http.Request) {
	setupResponse(&w, r)

	// Handle CORS preflight request
	if (*r).Method == "OPTIONS" {
		return
	}

	reqBody, _ := ioutil.ReadAll(r.Body)
	var data ClientData
	json.Unmarshal(reqBody, &data)

	songInfo := odesli(&data) // Get song info from Odesli

	if songInfo.SpotifyId != "" { // Use data from Odesli if data was found
		likeSpotifyTrack(&data, &songInfo)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(songInfo)
	} else { // Use AudD music recognition if music data not found with Odesli
		fmt.Println("no match found with Odesli.")
		downloadVideo(&data)         // download video using youtubedl
		convertVideo(&data)          // convert video to mp3 format
		songInfo = matchAudio(&data) // pass mp3 to audD api to perform music recognition
		deleteFile(&data, ".mp3")
		deleteFile(&data, ".mp4")

		if songInfo.SpotifyId != "" {
			likeSpotifyTrack(&data, &songInfo)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(songInfo)
		} else { // return json with error message if song info could not be found
			fmt.Println("no match found with Odesli or AudD")
			var errorMessage Error
			errorMessage.Error = "Failed to find song info"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(errorMessage)
		}
	}
}

// Add track to liked playlist in user's Spotify account
func likeSpotifyTrack(clientData *ClientData, trackData *SongData) (statusCode int) {
	client := &http.Client{}

	req, err := http.NewRequest(http.MethodPut, "https://api.spotify.com/v1/me/tracks?ids="+trackData.SpotifyId, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+clientData.AccessToken)

	response, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	statusCode = response.StatusCode
	return
}

// Download client specified video
func downloadVideo(clientData *ClientData) {
	fmt.Println("downloading...")
	videoId := strings.Split(clientData.VideoUrl, "?v=")[1]
	videoId2 := strings.Split(videoId, "&")[0]
	client := youtube.Client{}

	video, err := client.GetVideo(videoId2)
	if err != nil {
		panic(err)
	}

	formats := video.Formats.WithAudioChannels()
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		panic(err)
	}

	file, err := os.Create(videoId2 + ".mp4")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		panic(err)
	}
}

// Use AudD audio recognition to find song info from converted mp3
func matchAudio(clientData *ClientData) (auddData SongData) {
	fmt.Println("matching with AudD...")
	videoId := strings.Split(clientData.VideoUrl, "?v=")[1]
	videoId2 := strings.Split(videoId, "&")[0]

	client := audd.NewClient(os.Getenv("AUDDIO_API_KEY"))
	file, err := os.Open(videoId2 + ".mp3")
	if err != nil {
		panic(err)
	}
	result, err := client.Recognize(file, "spotify", nil)
	if err != nil {
		panic(err)
	}

	auddData.Title = result.Title
	auddData.Artist = result.Artist
	auddData.SpotifyId = result.Spotify.ID
	return
}

// Convert mp4 downloaded with youtubeDL into mp3 to be used by AudD
func convertVideo(clientData *ClientData) {
	fmt.Println("converting...")
	videoId := strings.Split(clientData.VideoUrl, "?v=")[1]
	videoId2 := strings.Split(videoId, "&")[0]

	err := fluentffmpeg.NewCommand(os.Getenv("FFMPEG_PATH")).
		InputPath(videoId2+".mp4").
		OutputOptions("-ss", "00:00:00", "-t", "00:00:24"). // starting time at beginning until 24 seconds in
		OutputFormat("mp3").
		OutputPath("./" + videoId2 + ".mp3").
		Run()
	if err != nil {
		panic(err)
	}
}

// Get song data from Odesli api
func odesli(data *ClientData) (odesliData SongData) {
	fmt.Println("matching with Odesli...")
	query := strings.Split(data.VideoUrl, "&")[0]
	odesliApiKey := os.Getenv("ODESLI_API_KEY")
	params := "https://api.song.link/v1-alpha.1/links?" +
		"url=" + url.QueryEscape(query) + "&" +
		"platform=youtube" + "&" +
		"key=" + url.QueryEscape(odesliApiKey)

	res, err := http.Get(params)
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()

	body, _ := ioutil.ReadAll(res.Body)
	var jsonData map[string]interface{}
	err2 := json.Unmarshal([]byte(body), &jsonData)
	if err2 != nil {
		panic(err2)
	}

	if (jsonData["linksByPlatform"].(map[string]interface{}))["spotify"] != nil {
		uniqueId := ((jsonData["linksByPlatform"].(map[string]interface{}))["spotify"].(map[string]interface{}))["entityUniqueId"].(string)

		odesliData.Title = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["title"].(string)
		odesliData.Artist = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["artistName"].(string)
		odesliData.SpotifyId = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["id"].(string)
		return
	}

	return
}

// Used to delete leftover mp3 and mp4 files
func deleteFile(clientData *ClientData, fileType string) {
	videoId := strings.Split(clientData.VideoUrl, "?v=")[1]
	videoId2 := strings.Split(videoId, "&")[0]

	err := os.Remove(videoId2 + fileType)
	if err != nil {
		panic(err)
	}
}

// Request handler
func handleRequests(mux *http.ServeMux) {
	mux.HandleFunc("/", homePageHandler)
	mux.HandleFunc("/api/like_song", likeSongHandler)
	log.Fatal(http.ListenAndServe(":"+os.Getenv("PORT"), mux))
}

// Set up response headers
func setupResponse(w *http.ResponseWriter, req *http.Request) {
	(*w).Header().Set("Access-Control-Allow-Origin", "*")
	(*w).Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT")
	(*w).Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
}

func main() {
	// Load env file
	err := godotenv.Load(".env")
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	// Run the server
	mux := http.NewServeMux()
	fmt.Println("Server running at localhost: " + os.Getenv("PORT"))
	handleRequests(mux)
}
