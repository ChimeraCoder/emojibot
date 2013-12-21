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
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"
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
	HTML_QUESTION_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2011-11-11/HTMLQuestion.xsd"
	FRAME_HEIGHT             = 100  // Frame height of the ExternalQuestion for MTurk, in pixels.
	ASSIGNMENT_DURATION      = 600  // How long, in seconds, a worker has to complete the assignment
	LIFETIME                 = 1200 // How long, in seconds, before the task expires
	MAX_ASSIGNMENTS          = 1    // Number of times the task needs to be performed
	AUTO_APPROVAL_DELAY      = 0    // Seconds before the request is auto-accepted. Set to 0 to accept immediately
	MAX_QUESTION_SIZE        = 65535
)

type HTMLQuestion struct {
	XMLName     xml.Name            `xml:"HTMLQuestion"`
	Xmlns       string              `xml:"xmlns,attr"`
	HTMLContent HTMLQuestionContent `xml:"HTMLContent"`
	FrameHeight int                 `xml:"FrameHeight"`
}

func (hq HTMLQuestion) XML() string {
	bs := make([]byte, 0, MAX_QUESTION_SIZE)
	bf := bytes.NewBuffer(bs)
	enc := xml.NewEncoder(bf)
	enc.Indent("  ", "    ")
	if err := enc.Encode(hq); err != nil {
		fmt.Printf("error: %v\n", err)
	}
	result, err := ioutil.ReadAll(bf)
	if err != nil {
		panic(err)
	}
	return string(result)
}

type HTMLQuestionContent struct {
	AssignmentId string
	Title        string
	Description  string
	ImageUrl     string
	Tweet        anaconda.Tweet
	TweetEmbed   anaconda.OEmbed
}

const HTMLQuestionTemplate = `{{define "T"}}<HTMLQuestion xmlns="http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2011-11-11/HTMLQuestion.xsd">
  <HTMLContent><![CDATA[
<!DOCTYPE html>
<html>
<head>
<meta http-equiv="Content-Type" content="text/html; charset=UTF-8"/>
<script type="text/javascript" src="https://s3.amazonaws.com/mturk-public/externalHIT_v1.js"></script>
</head>
<body>
<form name="mturk_form" method="post" id="mturk_form" action="https://www.mturk.com/mturk/externalSubmit">
<input type="hidden" value="" name="{{.AssignmentId}}" id="{{.AssignmentId}}"/>
<h2>{{.Title}}</h2>
<div style="border: 2px #000000 solid">
<h4>
{{.TweetEmbed.Html}}
</h4>
</div>
<div>
Pick the emoji that you feel would be the best translation of this tweet. For example, if the tweet were

<blockquote class="twitter-tweet" lang="en"><p>Call me Ishmael</p>&mdash; Aditya Mukerjee (@chimeracoder) <a href="https://twitter.com/chimeracoder/statuses/412631000544333824">December 16, 2013</a></blockquote>
<script async src="//platform.twitter.com/widgets.js" charset="utf-8"></script>
you might translate it as into the following emoji:
    <img src="{{.ImageUrl}}">
</div>
<p><textarea name="comment" cols="80" rows="3"></textarea></p>
<p><input type="submit" id="submitButton" value="Submit" /></p></form>
<script language="Javascript">turkSetAssignmentID();</script>
</body>
</html>
]]></HTMLContent>
<FrameHeight>450</FrameHeight>
</HTMLQuestion>{{end}}
`

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

type SearchHITsResponse struct {
	XMLName xml.Name `xml:"SearchHITsResponse"`
	Request struct {
		IsValid bool
	}
	NumResults      int
	TotalNumResults int
	PageNumber      int
	HIT             []struct {
		HITId                        string
		HITTypeId                    string
		CreationTime                 string
		Title                        string
		Description                  string
		HITStatus                    string
		Expiration                   string
		NumberOfAssignmentsPending   string
		NumberOfAssignmentsAvailable string
		NumberOfAssignmentsCompleted string
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
func CreateHIT(auth aws.Auth, title string, description string, htmlQuestionContent HTMLQuestionContent, rewardAmount string, rewardCurrencyCode string, assignmentDuration int, lifetime int, keywords []string, autoApprovalDelay int, requesterAnnotation string, uniqueRequestToken string, responseGroup string) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const service = "AWSMechanicalTurkRequester"
	const operation = "CreateHIT"

	t := time.Now()

	timestamp := t.Format(time.RFC3339)

	bs := make([]byte, 0, MAX_QUESTION_SIZE)
	bf := bytes.NewBuffer(bs)
	tmpl, err := template.New("foo").Parse(HTMLQuestionTemplate)
	if err != nil {
		panic(err)
		return nil, err
	}
	err = tmpl.ExecuteTemplate(bf, "T", htmlQuestionContent)
	if err != nil {
		panic(err)
		return nil, err
	}

	bts, err := ioutil.ReadAll(bf)
	if err != nil {
		panic(err)
		return nil, err
	}

	log.Printf("Bts are %s", string(bts))
	v := url.Values{}
	v.Set("AWSAccessKeyId", auth.AccessKey) //TODO set this
	v.Set("Version", "2012-03-25")
	v.Set("Operation", operation)
	v.Set("Description", description)
	v.Set("Timestamp", timestamp)
	v.Set("Title", title)
	//v.Set("Question", hq.XML())
	v.Set("Question", string(bts))
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
	bts, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Result was %s\n\n", string(bts))
	var result CreateHITResponse
	err = xml.Unmarshal(bts, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func SearchHIT(auth aws.Auth, v url.Values) (*SearchHITsResponse, error) {
	const OPERATION = "SearchHITs"
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const SERVICE = "AWSMechanicalTurkRequester"
	if v == nil {
		v = url.Values{}
	}

	timestamp := time.Now().Format(time.RFC3339)
	v.Set("AWSAccessKeyId", auth.AccessKey)
	v.Set("Operation", OPERATION)
	v.Set("Timestamp", timestamp)
	v.Set("AWSAccessKeyId", auth.AccessKey)

	sign(auth, SERVICE, OPERATION, timestamp, v)

	resp, err := http.PostForm(QUERY_URL, v)
	if err != nil {
		return nil, err
	}
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Result was %s\n\n", string(bts))
	var result SearchHITsResponse
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
	embed, err := a.GetOEmbedId(tweetId, nil)
	if err != nil {
		panic(err)
	}

	log.Print("Successfully obtained tweet")

	hq := HTMLQuestionContent{tweet.Id_str, title, description, "http://www.emojidick.com/emoji.png", tweet, embed}

	resp, err := CreateHIT(auth, title, description, hq, rewardAmount, rewardCurrencyCode, assignmentDuration, lifetime, keywords, autoApprovalDelay, tweet.Id_str, tweet.Id_str+time.Now().String(), responseGroup)
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

	title := `Translate tweet into emoji`
	description := `Pick the emoji that you feel would be the best translation of this tweet.`
	displayName := "How would you translate this tweet?"

	resp, err := CreateTranslationHIT(auth, a, 412631000544333824, title, description, displayName, "0.15", 120, 1200, HIT_KEYWORDS, 0)

	if err != nil {
		panic(err)
	}

	hitId := resp.HIT.HITId
	log.Print(resp)
	log.Printf("hitId is %s", hitId)

	<-time.After(5 * time.Second)

	// TODO expand this to work for more than 100 simultaneous outstanding tasks
	//hitsSearch, err := mt.SearchHITs()
	hitsSearch, err := SearchHIT(auth, nil)
	if err != nil {
		panic(err)
	}
	for _, hit := range hitsSearch.HIT {
		if hit.HITId == hitId {
			log.Printf(">>>>>>Status of HIT %s is %s", hitId, hit.NumberOfAssignmentsCompleted)
		} else {
			log.Printf("Received %v", hit)
		}
	}
	log.Printf("Length is %d", len(hitsSearch.HIT))
	log.Printf("Received %+v", hitsSearch)
}
