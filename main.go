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
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/joho/godotenv"
)

const (
	languageCode  = "cmn-CN"
	defaultName   = "cmn-CN-Chirp3-HD-Achernar"
	audioEncoding = "MP3"
	speakingRate  = 0.9
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

func isValidText(text string) bool {
	if utf8.RuneCountInString(text) > 5 {
		return false
	}
	// \\p{Han} is a Unicode property that matches Han characters.
	match, _ := regexp.MatchString(`^[\p{Han}]+$`, text)
	return match
}

func handleTTS(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	text := query.Get("text")
	if text == "" {
		http.Error(w, "Missing ?text= parameter", http.StatusBadRequest)
		return
	}

	if !isValidText(text) {
		http.Error(w, "Invalid text: must be all Chinese characters with a max length of 5", http.StatusBadRequest)
		return
	}

	modelName := query.Get("model")
	if modelName == "" {
		modelName = defaultName
	}

	reset := query.Get("reset") == "true"

	filename := sanitizeFilename(fmt.Sprintf("%s_%s", modelName, text)) + ".mp3"
	filePath := filepath.Join(outputDir, filename)

	// Skip cache if reset=true
	if !reset {
		if _, err := os.Stat(filePath); err == nil {
			log.Printf("Serving cached file: %s", filePath)
			w.Header().Set("Content-Type", "audio/mpeg")
			http.ServeFile(w, r, filePath)
			return
		}
	} else {
		log.Printf("Cache reset requested for: %s", text)
	}

	log.Printf("Generating new file for text: %s (model: %s)", text, modelName)

	apiURL := fmt.Sprintf("https://texttospeech.googleapis.com/v1/text:synthesize?key=%s", apiKey)
	payload := fmt.Sprintf(`{
		"input": {"text": %q},
		"voice": {"languageCode": "%s", "name": "%s"},
		"audioConfig": {"audioEncoding": "%s", "speakingRate": %.2f}
	}`, text, languageCode, modelName, audioEncoding, speakingRate)

	resp, err := http.Post(apiURL, "application/json", io.NopCloser(strings.NewReader(payload)))
	if err != nil {
		http.Error(w, "TTS request failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// log.Printf("Response body: %s", string(body)) // debug print

	var result struct {
		AudioContent string `json:"audioContent"`
		Error        any    `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
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
	if len([]rune(s)) > 50 {
		s = string([]rune(s)[:50])
	}
	return s
}
