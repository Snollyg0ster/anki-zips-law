package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

const (
	outputFolder  = "output"
	audioFolder   = "audio"
	inputFolder   = "input"
	meanings      = "meanings.json"
	lemma         = "lemma.txt"
	cardsFileName = "cards.txt"
)

func main() {
	txtFlag := flag.Bool("txt", false, "for parsing cypfr words in anki deck txt")
	audioFlag := flag.Int("audio", 0, "for generating audio fo N number of words")

	flag.Parse()

	if *txtFlag {
		createAnkiDeckTxt(lemma)
	}

	if *audioFlag != 0 {
		downloadIfNotExists("test", "test audio file")
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

type Meaning struct {
	Word    string `json:"w"`
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
	PartOfSpeach string
	Number       int
	Amount       int
}

func createAnkiDeckTxt(filename string) {
	f, err := os.Open(path.Join(inputFolder, filename))
	checkErr(err)
	defer f.Close()

	meaningsBytes, err := os.ReadFile(path.Join(inputFolder, meanings))
	checkErr(err)

	meanings := []Meaning{}
	checkErr(json.Unmarshal(meaningsBytes, &meanings))

	scanner := bufio.NewScanner(f)
	index := 0
	cards := []Card{}

	for scanner.Scan() {
		line := scanner.Text()
		lineArgs := strings.Split(line, " ")
		num, am, word, partOfSpeach := lineArgs[0], lineArgs[1], lineArgs[2], lineArgs[3]
		number, err := strconv.Atoi(num)

		checkErr(err)

		amount, err := strconv.Atoi(am)

		checkErr(err)
		id := fmt.Sprintf("%s-%s", lineArgs[2], lineArgs[3])
		cards = append(cards, Card{
			Meaning: Meaning{
				Word:    word,
				Meaning: meanings[index].Meaning,
				Example: meanings[index].Example,
			},
			Image:        fmt.Sprintf("%s.jpg", id),
			Sound:        fmt.Sprintf("%s.mp3", id),
			SoundMeaning: fmt.Sprintf("%s-meaning.mp3", id),
			SoundExample: fmt.Sprintf("%s-example.mp3", id),
			// TODO IPA
			PartOfSpeach: partsOfSpeech[partOfSpeach],
			Number:       number,
			Amount:       amount,
		})
		index++

		if index >= len(meanings) {
			break
		}
	}

	cardsFile, err := os.Create(path.Join(outputFolder, cardsFileName))

	checkErr(err)

	writer := bufio.NewWriter(cardsFile)
	_, err = writer.WriteString(strings.Join([]string{
		"Word",
		"Meaning",
		"Example",
		"Image",
		"Sound",
		"SoundMeaning",
		"SoundExample",
		"PartOfSpeach",
		"Number",
		"Amount",
		"\n",
	}, "; "))

	checkErr(err)

	for _, card := range cards {
		line := strings.Join(
			[]string{
				card.Word,
				card.Meaning.Meaning,
				card.Example,
				card.Image,
				card.Sound,
				card.SoundMeaning,
				card.SoundExample,
				card.PartOfSpeach,
				strconv.Itoa(card.Number),
				strconv.Itoa(card.Amount),
				"\n",
			},
			"; ",
		)
		_, err := writer.WriteString(line)
		checkErr(err)
	}
}

func downloadIfNotExists(fileName string, text string) error {
	f, err := os.Open(fileName + ".mp3")
	defer f.Close()

	if err != nil {
		url := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&total=1&idx=0&textlen=32&client=tw-ob&q=%s&tl=%s", url.QueryEscape(text), "en")
		response, err := http.Get(url)

		if err != nil {
			return err
		}

		defer response.Body.Close()

		output, err := os.Create(path.Join(outputFolder, audioFolder, fileName))

		if err != nil {
			return err
		}

		_, err = io.Copy(output, response.Body)
		return err
	}

	return nil
}

func checkErr(e error) {
	if e != nil {
		panic(e)
	}
}
