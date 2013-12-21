package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/gorilla/sessions"
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
	httpAddr            = flag.String("addr", ":8000", "HTTP server address")
	baseTmpl     string = "templates/base.tmpl"
	store               = sessions.NewCookieStore([]byte(COOKIE_SECRET)) //CookieStore uses secure cookies
	DOMAIN              = flag.String("domain", "", "The domain name where the site is being served")
	HIT_KEYWORDS        = []string{"twitter", "emoji"}

	//The following three variables can be defined using environment variables
	//to avoid committing them by mistake

	COOKIE_SECRET               = []byte(os.Getenv("COOKIE_SECRET"))
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

func queueTranslationHIT(tweetId int64) {
	eq := mturk.ExternalQuestion{xml.Name{}, *DOMAIN + fmt.Sprintf("/translate/tweets/%s", tweetId), FRAME_HEIGHT}
	log.Print(eq)
	type Price struct {
		Amount         string
		CurrencyCode   string
		FormattedPrice string
	}

	//ht, err := mt.CreateHIT("Translate Tweet into Emoji", "Please pick the emoji that best describe the following tweet", eq ExternalQuestion, reward, ASSIGNMENT_DURATION, LIFETIME, strings.Join(",", HIT_KEYWORDS), MAX_ASSIGNMENTS, nil)
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

func CreateTranslationHIT(a *anaconda.TwitterApi) *QuestionForm {
	auth, err := aws.EnvAuth()
	if err != nil {
		panic(err)
	}

	log.Print("About to request tweet")

	tweet, err := a.GetTweet(414197411058573312, nil)
	if err != nil {
		panic(err)
	}
	log.Print("Successfully fetched tweet %s", tweet.Text)

	answerSpecification := AnswerSpecification{FreeTextAnswer{Constraints{Length{1, 140}}, "", 1}}
	question := Question{tweet.Id_str, "Translating tweets into Emoji", true, QuestionContent{tweet.Text}, answerSpecification}
	qf := QuestionForm{Xmlns: QUESTION_FORM_SCHEMA_URL, Question: question}

	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"

	//reward := _Price{mturk.Price{"0.25", "USD", ""}}

	hit_description := "Please pick the emoji that would provide the best translation of this tweet"
	hit_title := "Translation of tweet into emoji"

	service := "AWSMechanicalTurkRequester"
	operation := "CreateHIT"

	t := time.Now()

	timestamp := t.Format(time.RFC3339)

	v := url.Values{}
	v.Set("AWSAccessKeyId", auth.AccessKey) //TODO set this
	v.Set("Version", "2012-03-25")
	v.Set("Operation", operation)
	v.Set("Description", hit_description)
	v.Set("Timestamp", timestamp)
	v.Set("Title", hit_title)
	v.Set("Question", qf.XML())
	v.Set("LifetimeInSeconds", strconv.Itoa(LIFETIME))
	v.Set("Reward.1.Amount", "0.25")
	v.Set("Reward.1.CurrencyCode", "USD")
	v.Set("AssignmentDurationInSeconds", strconv.Itoa(ASSIGNMENT_DURATION))
	v.Set("LifetimeInSeconds", strconv.Itoa(LIFETIME))
	v.Set("Keywords", strings.Join(HIT_KEYWORDS, ","))
	v.Set("AutoApprovalDelayInSeconds", strconv.Itoa(AUTO_APPROVAL_DELAY))
	v.Set("RequesterAnnotation", tweet.Id_str)
	v.Set("UniqueRequestToken", tweet.Id_str)
	v.Set("ResponseGroup", "Minimal")

	sign(auth, service, operation, timestamp, v)

	log.Printf("Values are %+v", v)
	resp, err := http.PostForm(QUERY_URL, v)
	if err != nil {
		panic(err)
	}
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	log.Print(string(bts))

	return &qf
}

func main() {
	anaconda.SetConsumerKey(string(TWITTER_CONSUMER_KEY))
	anaconda.SetConsumerSecret(string(TWITTER_CONSUMER_SECRET))

	a := anaconda.NewTwitterApi(string(TWITTER_ACCESS_TOKEN), string(TWITTER_ACCESS_TOKEN_SECRET))
	qf := CreateTranslationHIT(a)
	log.Print(qf)
}
