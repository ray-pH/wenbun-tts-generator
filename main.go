package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"github.com/joho/godotenv"
)

const (
	languageCode  = "cmn-CN"
	name          = "cmn-CN-Chirp3-HD-Achernar"
	audioEncoding = "MP3"
	speakingRate  = 1.0
)

var (
	apiKey    string
	outputDir string
)

func main() {
	_ = godotenv.Load()

	apiKey = os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		log.Fatal("Missing GOOGLE_API_KEY in .env")
	}

	outputDir = os.Getenv("OUTPUT_DIR")
	if outputDir == "" {
		outputDir = "./audio"
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output dir: %v", err)
	}

	http.HandleFunc("/tts", handleTTS)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server running at http://localhost:%s/tts?text=你好世界", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		http.Error(w, "Missing ?text= parameter", http.StatusBadRequest)
		return
	}

	filename := sanitizeFilename(text) + ".mp3"
	filePath := filepath.Join(outputDir, filename)

	// Check if the file already exists
	if _, err := os.Stat(filePath); err == nil {
		// File exists, serve it
		log.Printf("Serving existing file: %s", filePath)
		w.Header().Set("Content-Type", "audio/mpeg")
		http.ServeFile(w, r, filePath)
		return
	}

	// File does not exist, generate it
	log.Printf("Generating new file for text: %s", text)

	apiURL := fmt.Sprintf("https://texttospeech.googleapis.com/v1/text:synthesize?key=%s", apiKey)
	payload := fmt.Sprintf(`{
		"input": {"text": %q},
		"voice": {"languageCode": "%s", "name": "%s"},
		"audioConfig": {"audioEncoding": "%s", "speakingRate": %.2f}
	}`, text, languageCode, name, audioEncoding, speakingRate)

	resp, err := http.Post(apiURL, "application/json", io.NopCloser(strings.NewReader(payload)))
	if err != nil {
		http.Error(w, "TTS request failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var result struct {
		AudioContent string `json:"audioContent"`
		Error        any    `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		http.Error(w, "Failed to parse response: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if result.AudioContent == "" {
		http.Error(w, "No audio content in response", http.StatusInternalServerError)
		return
	}

	audio, err := base64.StdEncoding.DecodeString(result.AudioContent)
	if err != nil {
		http.Error(w, "Failed to decode audio: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Save the new file
	if err := os.WriteFile(filePath, audio, 0644); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Saved new file: %s", filePath)

	// Serve the newly created file
	w.Header().Set("Content-Type", "audio/mpeg")
	http.ServeFile(w, r, filePath)
}

// sanitizeFilename ensures filename is valid and short enough.
func sanitizeFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.TrimSpace(s)
	if len([]rune(s)) > 30 {
		s = string([]rune(s)[:30])
	}
	return s
}