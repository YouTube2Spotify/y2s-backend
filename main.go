package main

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/AudDMusic/audd-go"
	"github.com/joho/godotenv"
	"github.com/kkdai/youtube/v2"
	fluentffmpeg "github.com/modfy/fluent-ffmpeg"
)

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

	data := struct {
		Message string
	}{
		Message: "Hello World!",
	}

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

	// delete files from previous api call
	deleteFile("audio.mp3")
	deleteFile("video.mp4")

	reqBody, _ := ioutil.ReadAll(r.Body)
	var chromeExtensionData ClientData
	json.Unmarshal(reqBody, &chromeExtensionData)

	songInfo := odesli(&chromeExtensionData) // Get song info from Odesli

	if songInfo.SpotifyId != "" { // Use data from Odesli if data was found
		likeSpotifyTrack(&chromeExtensionData, &songInfo)
		likeStatus := likeSpotifyTrack(&chromeExtensionData, &songInfo)
		if likeStatus != 200 {
			errorHandler(&w, "Error liking song on Spotify", 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(songInfo)
	} else { // Use AudD music recognition if music data not found with Odesli
		log.Println("no match found with Odesli.")

		// begin download & time out if it still hasn't completed in 10 seconds
		channel := make(chan interface{})
		downloadStatus := true
		go func() {
			download := downloadVideo(&chromeExtensionData)
			channel <- download
		}()
		select {
		case downloadResponse := <-channel:
			log.Println(downloadResponse)
		case <-time.After(10 * time.Second):
			log.Println("download timed out")
			downloadStatus = false
		}

		// exit function early if video failed to download
		if !downloadStatus {
			errorHandler(&w, "Failed to download video", 500)
			return
		}

		convertVideo()                              // convert video to mp3 format
		songInfo = matchAudio(&chromeExtensionData) // pass mp3 to audD api to perform music recognition

		if songInfo.SpotifyId != "" {
			likeStatus := likeSpotifyTrack(&chromeExtensionData, &songInfo)
			if likeStatus != 200 {
				errorHandler(&w, "Error liking song on Spotify", 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(songInfo)
		} else { // return json with error message if song info could not be found
			log.Println("no match found with Odesli or AudD")
			errorHandler(&w, "Failed to find song info with both Odesli and AudD", 404)
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
	return statusCode
}

// Download client specified video
func downloadVideo(clientData *ClientData) (status string) {
	log.Println("downloading...")
	videoId := strings.Split(clientData.VideoUrl, "?v=")[1]
	videoId2 := strings.Split(videoId, "&")[0]
	client := youtube.Client{}

	video, err := client.GetVideo(videoId2)
	if err != nil {
		log.Panic(err)
	}

	formats := video.Formats.WithAudioChannels()
	stream, _, err := client.GetStream(video, &formats[0])
	if err != nil {
		log.Panic(err)
	}

	file, err := os.Create("video.mp4")
	if err != nil {
		log.Panic(err)
	}
	defer file.Close()

	_, err = io.Copy(file, stream)
	if err != nil {
		log.Panic(err)
	}

	status = "download complete"
	return status
}

// Use AudD audio recognition to find song info from converted mp3
func matchAudio(clientData *ClientData) SongData {
	log.Println("matching with AudD...")

	client := audd.NewClient(os.Getenv("AUDDIO_API_KEY"))
	file, err := os.Open("audio.mp3")
	if err != nil {
		log.Panic(err)
	}
	result, err := client.Recognize(file, "spotify", nil)
	if err != nil {
		log.Panic(err)
	}

	auddData := SongData{Title: result.Title, Artist: result.Artist}

	// check if Spotify data exists. appears to sometimes not exist
	switch spotifyId := result.Spotify; spotifyId {
	case nil:
		auddData.SpotifyId = ""
	default:
		auddData.SpotifyId = result.Spotify.ID
	}

	return auddData
}

// Convert mp4 downloaded with youtubeDL into mp3 to be used by AudD
func convertVideo() {
	log.Println("converting...")

	err := fluentffmpeg.NewCommand(os.Getenv("FFMPEG_PATH")).
		InputPath("video.mp4").
		OutputOptions("-ss", "00:00:00", "-t", "00:00:24"). // starting time at beginning until 24 seconds in
		OutputFormat("mp3").
		OutputPath("./audio.mp3").
		Run()
	if err != nil {
		log.Panic(err)
	}
}

// Get song data from Odesli api
func odesli(data *ClientData) (odesliData SongData) {
	log.Println("matching with Odesli...")
	query := strings.Split(data.VideoUrl, "&")[0]
	odesliApiKey := os.Getenv("ODESLI_API_KEY")
	params := "https://api.song.link/v1-alpha.1/links?" +
		"url=" + url.QueryEscape(query) + "&" +
		"platform=youtube" + "&" +
		"key=" + url.QueryEscape(odesliApiKey)

	res, err := http.Get(params)
	if err != nil {
		log.Panic(err)
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Println("Odesli error code: " + strconv.Itoa(res.StatusCode))
		return
	}

	body, _ := ioutil.ReadAll(res.Body)
	var jsonData map[string]interface{}
	if err2 := json.Unmarshal(body, &jsonData); err != nil {
		log.Panic(err2)
	}

	if (jsonData["linksByPlatform"].(map[string]interface{}))["spotify"] != nil {
		uniqueId := ((jsonData["linksByPlatform"].(map[string]interface{}))["spotify"].(map[string]interface{}))["entityUniqueId"].(string)

		odesliData.Title = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["title"].(string)
		odesliData.Artist = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["artistName"].(string)
		odesliData.SpotifyId = (jsonData["entitiesByUniqueId"].(map[string]interface{})[uniqueId].(map[string]interface{}))["id"].(string)
		return odesliData
	}

	return odesliData
}

// Used to delete leftover mp3 and mp4 files
func deleteFile(fileName string) {
	if err := os.Remove(fileName); err != nil {
		return // if file doesn't exist, do nothing
	}
}

func errorHandler(w *http.ResponseWriter, errMessage string, statusCode int) {
	errorMessage := Error{Error: errMessage}
	(*w).Header().Set("Content-Type", "application/json")
	(*w).WriteHeader(statusCode)
	json.NewEncoder(*w).Encode(errorMessage)
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
	if err := godotenv.Load(".env"); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Run the server
	mux := http.NewServeMux()
	log.Println("Server running at localhost: " + os.Getenv("PORT"))
	handleRequests(mux)
}
