package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"launchpad.net/goamz/exp/mturk"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	HIT_KEYWORDS = []string{"twitter", "emoji"}

	//The following variables can be defined using environment variables
	//to avoid committing them by mistake

	TWITTER_CONSUMER_KEY        = []byte(os.Getenv("TWITTER_CONSUMER_KEY"))
	TWITTER_CONSUMER_SECRET     = []byte(os.Getenv("TWITTER_CONSUMER_SECRET"))
	TWITTER_ACCESS_TOKEN        = []byte(os.Getenv("TWITTER_ACCESS_TOKEN"))
	TWITTER_ACCESS_TOKEN_SECRET = []byte(os.Getenv("TWITTER_ACCESS_TOKEN_SECRET"))
)

const (
	QUESTION_FORM_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2005-10-01/QuestionForm.xsd"
	FRAME_HEIGHT             = 100  // Frame height of the ExternalQuestion for MTurk, in pixels.
	ASSIGNMENT_DURATION      = 600  // How long, in seconds, a worker has to complete the assignment
	LIFETIME                 = 1200 // How long, in seconds, before the task expires
	MAX_ASSIGNMENTS          = 1    // Number of times the task needs to be performed
	AUTO_APPROVAL_DELAY      = 0    // Seconds before the request is auto-accepted. Set to 0 to accept immediately
	MAX_QUESTION_SIZE        = 65535
)

type QuestionForm struct {
	XMLName xml.Name `xml:"QuestionForm"`
	Xmlns   string   `xml:"xmlns,attr"`
	//Overview string   `xml:"Overview"`
	Question Question
}

func (qf QuestionForm) XML() string {
	bs := make([]byte, 0, MAX_QUESTION_SIZE)
	bf := bytes.NewBuffer(bs)
	enc := xml.NewEncoder(bf)
	enc.Indent("  ", "    ")
	if err := enc.Encode(qf); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	result, err := ioutil.ReadAll(bf)
	if err != nil {
		panic(err)
	}
	return string(result)
}

type Question struct {
	QuestionIdentifier  string              `xml:"QuestionIdentifier"`
	DisplayName         string              `xml:"DisplayName"`
	IsRequired          bool                `xml:"IsRequired"`
	QuestionContent     QuestionContent     `xml:"QuestionContent"`
	AnswerSpecification AnswerSpecification `xml:"AnswerSpecification",omitempty`
}

type QuestionContent struct {
	Text string `xml:"Text"`
}

type AnswerSpecification struct {
	FreeTextAnswer FreeTextAnswer `xml:"FreeTextAnswer"`
}

type FreeTextAnswer struct {
	Constraints             Constraints `xml:"Constraints"`
	DefaultText             string      `xml:"DefaultText"`
	NumberOfLinesSuggestion int         `xml:"NumberOfLinesSuggestion"`
}

type Constraints struct {
	//IsNumeric IsNumeric `xml:"IsNumeric"`
	Length Length `xml:"Length"`
}

type Length struct {
	MinLength int `xml:"minLength,attr"`
	MaxLength int `xml:"maxLength,attr"`
	//AnswerFormatRegex AnswerFormatRegex `xml:"AnswerFormatRegex"`
}

type HIT struct {
	HITId        string
	HITTYpeId    string
	CreationTime string
	HitStatus    string
}

type CreateHITResponse struct {
	XMLName          xml.Name `xml:"CreateHITResponse"`
	OperationRequest struct {
		RequestId string
	}
	HIT struct {
		Request struct {
			IsValid bool
		}
		HITId     string
		HITTypeId string
	}
}

func sign(auth aws.Auth, service, method, timestamp string, v url.Values) {
	b64 := base64.StdEncoding
	payload := service + method + timestamp
	hash := hmac.New(sha1.New, []byte(auth.SecretKey))
	hash.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(hash.Size()))
	b64.Encode(signature, hash.Sum(nil))
	v.Set("Signature", string(signature))
}

// Create an HIT
func CreateHIT(auth aws.Auth, title string, description string, questionForm QuestionForm, rewardAmount string, rewardCurrencyCode string, assignmentDuration int, lifetime int, keywords []string, autoApprovalDelay int, requesterAnnotation string, uniqueRequestToken string, responseGroup string) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const service = "AWSMechanicalTurkRequester"
	const operation = "CreateHIT"

	t := time.Now()

	timestamp := t.Format(time.RFC3339)

	v := url.Values{}
	v.Set("AWSAccessKeyId", auth.AccessKey) //TODO set this
	v.Set("Version", "2012-03-25")
	v.Set("Operation", operation)
	v.Set("Description", description)
	v.Set("Timestamp", timestamp)
	v.Set("Title", title)
	v.Set("Question", questionForm.XML())
	v.Set("LifetimeInSeconds", strconv.Itoa(lifetime))
	v.Set("Reward.1.Amount", rewardAmount)
	v.Set("Reward.1.CurrencyCode", rewardCurrencyCode)
	v.Set("AssignmentDurationInSeconds", strconv.Itoa(assignmentDuration))
	v.Set("Keywords", strings.Join(keywords, ","))
	v.Set("AutoApprovalDelayInSeconds", strconv.Itoa(autoApprovalDelay))
	v.Set("RequesterAnnotation", requesterAnnotation)
	v.Set("UniqueRequestToken", uniqueRequestToken)
	v.Set("ResponseGroup", responseGroup)

	sign(auth, service, operation, timestamp, v)

	resp, err := http.PostForm(QUERY_URL, v)
	if err != nil {
		return nil, err
	}
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result CreateHITResponse
	err = xml.Unmarshal(bts, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func CreateTranslationHIT(auth aws.Auth, a *anaconda.TwitterApi, tweetId int64, title string, description string, displayName string, rewardAmount string, assignmentDuration int, lifetime int, keywords []string, autoApprovalDelay int) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const service = "AWSMechanicalTurkRequester"
	const operation = "CreateHIT"
	const rewardCurrencyCode = "USD" // This is the only one supported for now by Amazon, anyway
	const responseGroup = "Minimal"

	auth, err := aws.EnvAuth()
	if err != nil {
		panic(err)
	}

	log.Print("About to request tweet")

	tweet, err := a.GetTweet(tweetId, nil)
	if err != nil {
		panic(err)
	}

	log.Print("Successfully obtained tweet")

	answerSpecification := AnswerSpecification{FreeTextAnswer{Constraints{Length{1, 140}}, "", 1}}
	question := Question{tweet.Id_str, displayName, true, QuestionContent{tweet.Text}, answerSpecification}
	questionForm := QuestionForm{Xmlns: QUESTION_FORM_SCHEMA_URL, Question: question}

	resp, err := CreateHIT(auth, title, description, questionForm, rewardAmount, rewardCurrencyCode, assignmentDuration, lifetime, keywords, autoApprovalDelay, tweet.Id_str, tweet.Id_str, responseGroup)
	return resp, err
}

func main() {

	auth, err := aws.EnvAuth()
	if err != nil {
		panic(err)
	}
	anaconda.SetConsumerKey(string(TWITTER_CONSUMER_KEY))
	anaconda.SetConsumerSecret(string(TWITTER_CONSUMER_SECRET))
	a := anaconda.NewTwitterApi(string(TWITTER_ACCESS_TOKEN), string(TWITTER_ACCESS_TOKEN_SECRET))

	resp, err := CreateTranslationHIT(auth, a, 414194572374204416, "Pick Emoji that translate a tweet", "Pick the emoji that you feel would be the best translation of this tweet. For example: 'Call me Ishmael' might be translated as '☎ 👨 ⛵ 🐳 📌'", "Please translate this tweet", "0.25", 120, 1200, HIT_KEYWORDS, 0)

	hitId := resp.HIT.HITId

	<-time.After(5 * time.Second)
	mt := mturk.New(auth)

	// TODO expand this to work for more than 100 simultaneous outstanding tasks
	hitsSearch, err := mt.SearchHITs()
	if err != nil {
		panic(err)
	}
	for _, hit := range hitsSearch.HITs {
		if hit.HITId == hitId {
			log.Printf("Status of HIT %s is %s", hitId, hit.HITReviewStatus)
		}
	}
}
