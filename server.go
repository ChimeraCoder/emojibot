package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"github.com/ChimeraCoder/anaconda"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
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
	AWS_SQS_URL                 = os.Getenv("AWS_SQS_URL")

	twitterBot *anaconda.TwitterApi
	awsAuth    *aws.Auth
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
	HITTYPE_ID               = "2KZXBAT2D5NQ20NW5CRJO5TYMZL26K"
)

func sign(auth aws.Auth, service, method, timestamp string, v url.Values) {
	b64 := base64.StdEncoding
	payload := service + method + timestamp
	hash := hmac.New(sha1.New, []byte(auth.SecretKey))
	hash.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(hash.Size()))
	b64.Encode(signature, hash.Sum(nil))
	v.Set("Signature", string(signature))
}

func sign2(auth aws.Auth, method, path string, params url.Values, host string) {
	b64 := base64.StdEncoding
	params.Set("AWSAccessKeyId", auth.AccessKey)
	params.Set("SignatureVersion", "2")
	params.Set("SignatureMethod", "HmacSHA256")

	var sarray []string
	for k, v := range params {
		sarray = append(sarray, aws.Encode(k)+"="+aws.Encode(v[0]))
	}
	sort.StringSlice(sarray).Sort()
	joined := strings.Join(sarray, "&")
	payload := method + "\n" + host + "\n" + path + "\n" + joined
	hash := hmac.New(sha256.New, []byte(auth.SecretKey))
	hash.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(hash.Size()))
	b64.Encode(signature, hash.Sum(nil))
	params.Set("Signature", string(signature))
}

func GetAssignmentsForHITOperation(hitID string) (*GetAssignmentsForHITResult, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const OPERATION = "GetAssignmentsForHIT"
	const service = "AWSMechanicalTurkRequester"
	v := url.Values{}
	timestamp := time.Now().Format(time.RFC3339)
	v.Set("HITId", hitID)
	v.Set("Version", "2012-03-25")
	v.Set("AWSAccessKeyId", awsAuth.AccessKey) //TODO set this
	v.Set("Operation", OPERATION)
	v.Set("Timestamp", timestamp)
	sign(*awsAuth, service, OPERATION, timestamp, v)
	resp, err := http.PostForm(QUERY_URL, v)
	if err != nil {
		return nil, err
	}
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result GetAssignmentsForHITResult
	err = xml.Unmarshal(bts, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Create an HIT
func CreateHIT(title string, description string, htmlQuestionContent HTMLQuestionContent, rewardAmount string, rewardCurrencyCode string, assignmentDuration int, lifetime int, keywords []string, autoApprovalDelay int, requesterAnnotation string, uniqueRequestToken string, responseGroup string) (*CreateHITResponse, error) {
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
	v.Set("AWSAccessKeyId", awsAuth.AccessKey) //TODO set this
	v.Set("Version", "2012-03-25")
	v.Set("Operation", operation)
	v.Set("HITTypeId", HITTYPE_ID)
	v.Set("Description", description)
	v.Set("Timestamp", timestamp)
	v.Set("Title", title)
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

	sign(*awsAuth, service, operation, timestamp, v)

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

func SetHITTypeNotification(hitTypeId string, notification Notification) (*SearchHITsResponse, error) {
	const OPERATION = "SetHITTypeNotification"
	const EVENT_TYPE = "AssignmentSubmitted"
	//Notification{AWS_SQS_URL, "SQS", "2006-05-05", "AssignmentSubmitted"}
	return nil, nil
}

// Receive at most one message from the queue
func ReceiveMessage() (*ReceiveMessageResponse, error) {
	QUERY_URL := AWS_SQS_URL
	const ACTION = "ReceiveMessage"

	timestamp := time.Now().Format(time.RFC3339)
	v := url.Values{}
	v.Set("Action", ACTION)
	v.Set("MaxNumberOfMessages", "1")
	v.Set("VisibilityTimeout", "15")
	v.Set("AttributeName", "All")
	v.Set("Timestamp", timestamp)
	v.Set("Version", "2009-02-01")
	v.Set("SignatureMethod", "HmacSHA1")
	v.Set("AWSAccessKeyId", awsAuth.AccessKey)
	v.Set("SignatureVersion", "2")

	sign2(*awsAuth, "POST", "/899223851996/EmojibotQueue", v, "sqs.us-east-1.amazonaws.com")
	resp, err := http.PostForm(QUERY_URL, v)
	if err != nil {
		return nil, err
	}
	bts, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Result was %s\n\n", string(bts))
	var result ReceiveMessageResponse
	err = xml.Unmarshal(bts, &result)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func SearchHIT(v url.Values) (*SearchHITsResponse, error) {
	const OPERATION = "SearchHITs"
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const SERVICE = "AWSMechanicalTurkRequester"
	if v == nil {
		v = url.Values{}
	}

	timestamp := time.Now().Format(time.RFC3339)
	v.Set("AWSAccessKeyId", awsAuth.AccessKey)
	v.Set("Operation", OPERATION)
	v.Set("Timestamp", timestamp)
	v.Set("AWSAccessKeyId", awsAuth.AccessKey)

	sign(*awsAuth, SERVICE, OPERATION, timestamp, v)

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

func CreateTranslationHIT(a *anaconda.TwitterApi, tweetId int64, title string, description string, displayName string, rewardAmount string, assignmentDuration int, lifetime int, keywords []string, autoApprovalDelay int) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const service = "AWSMechanicalTurkRequester"
	const operation = "CreateHIT"
	const rewardCurrencyCode = "USD" // This is the only one supported for now by Amazon, anyway
	const responseGroup = "Minimal"

	log.Print("About to request tweet")

	tweet, err := a.GetTweet(tweetId, nil)
	if err != nil {
		return nil, err
	}
	embed, err := a.GetOEmbedId(tweetId, nil)
	if err != nil {
		return nil, err
	}

	log.Print("Successfully obtained tweet")

	hq := HTMLQuestionContent{tweet.Id_str, title, description, "http://www.emojidick.com/emoji.png", tweet, embed}

	resp, err := CreateHIT(title, description, hq, rewardAmount, rewardCurrencyCode, assignmentDuration, lifetime, keywords, autoApprovalDelay, tweet.Id_str, tweet.Id_str+time.Now().String(), responseGroup)
	return resp, err
}

func main() {

	if tmp, err := aws.EnvAuth(); err != nil {
		panic(err)
	} else {
		awsAuth = &tmp
	}

	result, err := ReceiveMessage()
	if err != nil {
		panic(err)
	}
	log.Printf("Result was %+v", result)
	log.Printf("ReceiveMessageResult %+v", result.ReceiveMessageResult.Message.Body)

	return

	anaconda.SetConsumerKey(string(TWITTER_CONSUMER_KEY))
	anaconda.SetConsumerSecret(string(TWITTER_CONSUMER_SECRET))
	twitterBot = anaconda.NewTwitterApi(string(TWITTER_ACCESS_TOKEN), string(TWITTER_ACCESS_TOKEN_SECRET))

	title := `Translate tweet into emoji`
	description := `Pick the emoji that you feel would be the best translation of this tweet.`
	displayName := "How would you translate this tweet?"

	resp, err := CreateTranslationHIT(twitterBot, 412631000544333824, title, description, displayName, "0.15", 120, 1200, HIT_KEYWORDS, 0)

	if err != nil {
		panic(err)
	}

	hitId := resp.HIT.HITId
	log.Print(resp)
	log.Printf("hitId is %s", hitId)

	<-time.After(5 * time.Second)

	// TODO expand this to work for more than 100 simultaneous outstanding tasks
	//hitsSearch, err := mt.SearchHITs()
	hitsSearch, err := SearchHIT(nil)
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
