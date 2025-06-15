package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

const (
	outputFolder          = "output"
	audioFolder           = "audio"
	imgFolder             = "img"
	inputFolder           = "input"
	meaningsFileName      = "meanings.json"
	lemmaFileName         = "lemma.txt"
	cardsFileName         = "cards.txt"
	ipasFileName          = "ipas.json"
	meaningsRequestAmount = 200
)

func main() {
	f, _ := ModifyLoggingOutput()
	defer f.Close()
	checkErr(godotenv.Load(".env"))

	txtFlag := flag.Bool("txt", false, "for parsing cypfr words in anki deck txt")
	audioFlag := flag.Bool("audio", false, "for generating audio for N number of words")
	meanings := flag.Bool("meanings", false, "for generating meanings of words")
	img := flag.Bool("img", false, "for generating img from examples")
	ipas := flag.Bool("ipas", false, "for generating ipas for words")

	flag.Parse()

	if *txtFlag {
		createAnkiDecksTxt()
	}

	if *audioFlag {
		generateAudio()
	}

	if *meanings {
		generateMeanings()
	}

	if *img {
		generateImgs()
	}

	if *ipas {
		getPhonetics()
	}
}

var partsOfSpeech = map[string]string{
	"n":                 "noun",
	"v":                 "verb",
	"a":                 "adjective",
	"adv":               "adverb",
	"conj":              "conjunction",
	"interjection":      "interjection",
	"pron":              "pronoun",
	"prep":              "preposition",
	"modal":             "modal verb",
	"co":                "coordinating conjunction",
	"det":               "determiner",
	"infinitive-marker": "infinitive marker",
}

type Word struct {
	Word         string `json:"w"`
	PartOfSpeach string `json:"p"`
}

type Meaning struct {
	Word
	Meaning string `json:"m"`
	Example string `json:"e"`
}

type Card struct {
	Meaning
	Image        string
	Sound        string
	SoundMeaning string
	SoundExample string
	IPA          string
	Number       int
	Amount       int
}

func parseLemma() []Card {
	f, err := os.Open(path.Join(inputFolder, lemmaFileName))
	checkErr(err)
	defer f.Close()

	meanings := *parseJson([]Meaning{}, inputFolder, meaningsFileName)
	meaningsMap := make(map[string]Meaning, 0)

	for _, meaning := range meanings {
		meaningsMap[meaning.Id()] = meaning
	}

	scanner := bufio.NewScanner(f)
	cards := []Card{}

	for scanner.Scan() {
		line := scanner.Text()
		lineArgs := strings.Split(line, " ")
		num, am, word, partOfSpeach := lineArgs[0], lineArgs[1], lineArgs[2], lineArgs[3]
		number, err := strconv.Atoi(num)

		checkErr(err)

		amount, err := strconv.Atoi(am)

		checkErr(err)
		id := getWordId(lineArgs[2], lineArgs[3])
		cards = append(cards, Card{
			Meaning: Meaning{
				Word: Word{
					Word:         word,
					PartOfSpeach: partsOfSpeech[partOfSpeach],
				},
				Meaning: meaningsMap[id].Meaning,
				Example: meaningsMap[id].Example,
			},
			Image:        fmt.Sprintf("%s.jpg", id),
			Sound:        fmt.Sprintf("%s-word.mp3", id),
			SoundMeaning: fmt.Sprintf("%s-meaning.mp3", id),
			SoundExample: fmt.Sprintf("%s-example.mp3", id),
			// TODO IPA
			Number: number,
			Amount: amount,
		})
	}

	return cards
}

func createAnkiDecksTxt() {
	cards := parseLemma()
	ipas := *parseJson(map[string]string{}, inputFolder, ipasFileName)

	sort.Slice(cards, func(i, j int) bool {
		return cards[i].Amount > cards[j].Amount
	})

	totalFrequency := 0
	for _, card := range cards {
		totalFrequency += card.Amount
	}

	deckFrequency := int(float64(totalFrequency) * 0.3)
	deckIndex := 1
	currentDeckCards := []Card{}
	currentDeckTotal := 0

	sound := func(name string) string {
		return fmt.Sprintf("[sound:%s]", name)
	}

	writeDeckToFile := func(deckCards []Card, deckNum int, deckTotal int) {
		if len(deckCards) == 0 {
			return
		}

		deckFileName := fmt.Sprintf("cards_deck_%d.txt", deckNum)
		cardsFile, err := os.Create(path.Join(outputFolder, deckFileName))
		checkErr(err)

		writer := bufio.NewWriter(cardsFile)

		for _, card := range deckCards {
			line := strings.Join(
				[]string{
					card.Id(),
					card.Word.Word,
					card.Meaning.Meaning,
					card.Example,
					fmt.Sprintf("<img src='%s'>", card.Image),
					sound(card.Sound),
					sound(card.SoundMeaning),
					sound(card.SoundExample),
					card.PartOfSpeach,
					ipas[card.Id()],
					strconv.Itoa(card.Number),
					strconv.Itoa(card.Amount),
				},
				"	",
			)
			_, err := writer.WriteString(line + "\n")
			checkErr(err)
		}

		writer.Flush()
		cardsFile.Close()

		fmt.Printf("Создана колода %d: %d карточек, общая частотность: %d (%.1f%% от общей)\n",
			deckNum, len(deckCards), deckTotal, float64(deckTotal)/float64(totalFrequency)*100)
	}

	for _, card := range cards {
		if currentDeckTotal+card.Amount > deckFrequency && len(currentDeckCards) > 0 {
			writeDeckToFile(currentDeckCards, deckIndex, currentDeckTotal)
			currentDeckCards = []Card{}
			currentDeckTotal = 0
			deckIndex++
		}

		currentDeckCards = append(currentDeckCards, card)
		currentDeckTotal += card.Amount
	}

	if len(currentDeckCards) > 0 {
		writeDeckToFile(currentDeckCards, deckIndex, currentDeckTotal)
	}
}

func generateMeanings() {
	existingMeanings := *parseJson([]Meaning{}, inputFolder, meaningsFileName)

	existingMeaningsMap := make(map[string]Meaning, 0)

	for _, existingMeaning := range existingMeanings {
		existingMeaningsMap[existingMeaning.Id()] = existingMeaning
	}

	wordsToTranslateMap := make(map[string]Word)
	wordsToTranslate := make([]Word, 0)
	file, err := os.Open(path.Join(inputFolder, lemmaFileName))
	checkErr(err)
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		lineArgs := strings.Split(line, " ")
		wordName, partOfSpeach := lineArgs[2], lineArgs[3]
		key := getWordId(wordName, partOfSpeach)
		word := Word{
			Word:         wordName,
			PartOfSpeach: partOfSpeach,
		}

		if _, ok := existingMeaningsMap[key]; ok {
			continue
		}

		wordsToTranslateMap[key] = word
		wordsToTranslate = append(wordsToTranslate, word)
	}

	// fmt.Println(wordsToTranslate[offset : offset+meaningsRequestAmount])
	for offset := 0; offset <= len(wordsToTranslate); offset += meaningsRequestAmount {
		words := wordsToTranslate[offset : offset+meaningsRequestAmount]
		fmt.Println(offset, offset+meaningsRequestAmount, words[0], words[len(words)-1])

		newMeanings := getDeepseekMeanings(words)

		savedMeanings := *parseJson([]Meaning{}, inputFolder, meaningsFileName)
		writeJson(append(savedMeanings, *newMeanings...), inputFolder, meaningsFileName)
	}
}

type DeepseekResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
}

type Message struct {
	Content string `json:"content"`
}

func getDeepseekMeanings(words []Word) *[]Meaning {
	client := &http.Client{
		Timeout: 10 * time.Minute,
	}

	wordsToTranslate := strings.Builder{}

	for _, word := range words {
		wordsToTranslate.WriteString(fmt.Sprintf("%s %s\\n", word.Word, word.PartOfSpeach))
	}

	payload := `{
		  "model": "deepseek/deepseek-chat-v3-0324:free",
		  "messages": [
			{
			  "role": "system",
			  "content": "%s"
			},
			{
			  "role": "user",
			  "content": "%s"
			}
		  ]
		}`
	systemPrompt := `I have words in format \"the det\". where the - word, det - part of speech. You should write meaning and example of this word considering part of speech. For example [{\"w\": \"the\", \"m\": \"Denoting one or more people or things already mentioned or assumed to be common knowledge\", \"e\": \"What's the matter?\", \"p\": \"det\"}]. All words please, delete whitespaces. Output should be in json format. Do not include any other text. I need it for further parsing`
	payloadReader := strings.NewReader(fmt.Sprintf(payload, systemPrompt, wordsToTranslate.String()))
	request, err := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions", payloadReader)

	checkErr(err)

	request.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("OPENTOUTER_API_TOKEN")))
	request.Header.Add("Content-Type", "application/json")
	log.Println(request, fmt.Sprintf(payload, systemPrompt, wordsToTranslate.String()))

	res, err := client.Do(request)
	checkErr(err)
	defer res.Body.Close()

	fmt.Println(res.Status)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error reading response body: %v", err)
	}

	log.Printf("Response body size: %d bytes", len(body))
	log.Println("Row response", string(body))

	checkErr(err)

	var response DeepseekResponse

	checkErr(json.Unmarshal(body, &response))

	log.Println("Response", response.Choices[0].Message.Content)

	content := response.Choices[0].Message.Content
	answer := strings.TrimPrefix(strings.Trim(content, "`"), "json")
	var meanings []Meaning

	checkErr(json.Unmarshal([]byte(answer), &meanings))

	return &meanings
}

func parseJson[T any](data T, filePath ...string) *T {
	bytes, err := os.ReadFile(path.Join(filePath...))

	if err != nil {
		return &data
	}

	json.Unmarshal(bytes, &data)

	return &data
}

func writeJson(data any, filePath ...string) {
	f, err := os.Create(path.Join(filePath...))
	checkErr(err)

	bytes, err := json.MarshalIndent(data, "", "	")
	checkErr(err)

	_, err = f.Write(bytes)
	checkErr(err)
}

func generateAudio() {
	existingMeanings := *parseJson([]Meaning{}, inputFolder, meaningsFileName)

	for index, meaning := range existingMeanings {

		for key, value := range map[string]string{"-meaning": meaning.Meaning, "-example": meaning.Example, "-word": meaning.Word.Word} {
			err, exist := downloadAudioIfNotExists(meaning.Id()+key, value)

			if err != nil {
				panic(err)
			}

			if !exist {
				fmt.Println(index, meaning.Id()+key, value)
			}
		}
	}
}

func downloadAudioIfNotExists(fileName string, text string) (error, bool) {
	exist := true
	filePath := path.Join(outputFolder, audioFolder, fileName+".mp3")
	f, err := os.Open(filePath)

	if err != nil {
		exist := false
		url := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&total=1&idx=0&textlen=32&client=tw-ob&q=%s&tl=%s", url.QueryEscape(text), "en")
		response, err := http.Get(url)

		if err != nil {
			return err, exist
		}

		defer response.Body.Close()

		output, err := os.Create(filePath)

		if err != nil {
			return err, exist
		}

		_, err = io.Copy(output, response.Body)
		return err, exist
	}

	f.Close()

	return nil, exist
}

func generateImgs() {
	existingMeanings := *parseJson([]Meaning{}, inputFolder, meaningsFileName)
	var wg sync.WaitGroup
	const (
		concurrency = 10
		windowSize  = 10
	)
	meaningsWithoutImg := []Meaning{}

	for _, meaning := range existingMeanings {
		if _, err := os.Stat(path.Join(outputFolder, imgFolder, meaning.Id()+".jpg")); os.IsNotExist(err) {
			meaningsWithoutImg = append(meaningsWithoutImg, meaning)
		}
	}

	downloadImgs := func(meanings []Meaning, offset int, count int) {
		for index, meaning := range meanings {
			err, exist := downloadImgIfNotExists(meaning.Id(), meaning.Example, count)

			if err != nil {
				fmt.Println(err)
				continue
			}

			if !exist {
				fmt.Printf("|%d| %d %s %s\n", count, offset+index, meaning.Id(), meaning.Example)
			}
		}
	}

	for i := 0; i < len(meaningsWithoutImg); i += windowSize * concurrency {
		fmt.Println("window", i, i+windowSize*concurrency)

		for j := range concurrency {
			start := i + j*windowSize
			end := i + (j+1)*windowSize

			if start >= len(meaningsWithoutImg) {
				break
			}

			if end > len(meaningsWithoutImg) {
				end = len(meaningsWithoutImg)
			}

			wg.Add(1)

			go func() {
				fmt.Println("goroutine", start, end)
				downloadImgs(meaningsWithoutImg[start:end], start, j)
				wg.Done()
			}()
		}

		wg.Wait()
	}
}

func getProxyClient(index int) *http.Client {
	// proxies := []string{
	// 	"http://23.254.229.117:17407",
	// }

	// proxyURL, _ := url.Parse(proxies[rand.Intn(len(proxies))])
	url, _ := url.Parse("http://23.254.229.117:17407")
	transport := &http.Transport{
		Proxy: http.ProxyURL(url),
	}

	if index == 0 {
		return &http.Client{
			Timeout: 90 * time.Second,
		}
	}

	return &http.Client{
		Transport: transport,
		Timeout:   90 * time.Second,
	}
}

func downloadImgIfNotExists(fileName string, text string, index int) (error, bool) {
	exist := true
	filePath := path.Join(outputFolder, imgFolder, fileName+".jpg")
	f, err := os.Open(filePath)

	if err != nil {
		exist := false
		url := fmt.Sprintf("https://image.pollinations.ai/prompt/%s", url.QueryEscape(text))
		req, _ := http.NewRequest("GET", url, nil)
		client := getProxyClient(index % 2)
		response, err := client.Do(req)

		if err != nil {
			for range 15 {
				response, err = client.Do(req)
			}

			if err != nil {
				return err, exist
			}
		}

		if response.StatusCode != 200 {
			response, _ := io.ReadAll(response.Body)

			fmt.Println(string(response))
			return nil, true
		}

		defer response.Body.Close()

		output, err := os.Create(filePath)

		if err != nil {
			return err, exist
		}

		_, err = io.Copy(output, response.Body)
		return err, exist
	}

	f.Close()

	return nil, exist
}

type Phonetic struct {
	Phonetic  string `json:"phonetic,omitempty"`
	Phonetics []Text `json:"phonetics"`
}

type Text struct {
	Text string `json:"text,omitempty"`
}

func getPhonetics() {
	words := parseLemma()
	ipas := *parseJson(map[string]string{}, inputFolder, ipasFileName)
	fmt.Println(len(words))

	for _, word := range words {
		if _, ok := ipas[word.Id()]; ok {
			continue
		}

		phonetic, err := getPhonetic(word.Word.Word)

		if err != nil {
			fmt.Println(word.Word.Word, "- cant find phonetic because")
			fmt.Println(err)
		}

		ipas[word.Id()] = phonetic
		fmt.Println(word.Word.Word, "- found phonetic", phonetic)
		writeJson(ipas, inputFolder, ipasFileName)
		time.Sleep(400 * time.Millisecond)
	}

}

func getPhonetic(word string) (string, error) {
	res, err := http.Get(fmt.Sprintf("https://api.dictionaryapi.dev/api/v2/entries/en/%s", word))

	if res.StatusCode != 200 {
		return "", fmt.Errorf("status code %d", res.StatusCode)
	}

	if err != nil {
		return "", err
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)

	if err != nil {
		return "", err
	}

	var phonetics []Phonetic
	// fmt.Println(string(body))
	err = json.Unmarshal(body, &phonetics)

	if err != nil {
		return "", err
	}

	phonetic := phonetics[0]

	if phonetic.Phonetic != "" {
		return phonetic.Phonetic, nil
	}

	for _, phonetic := range phonetic.Phonetics {
		if phonetic.Text != "" {
			return phonetic.Text, nil
		}
	}

	return "", nil
}

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}

func (p *Meaning) Id() string {
	return getWordId(p.Word.Word, p.PartOfSpeach)
}

func getWordId(word string, partOfSpeach string) string {
	return fmt.Sprintf("%s-%s", word, partOfSpeach)
}
