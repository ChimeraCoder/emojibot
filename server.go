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

	twitterBot *anaconda.TwitterApi
	awsAuth    *aws.Auth
)

const (
	QUESTION_FORM_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2005-10-01/QuestionForm.xsd"
	HTML_QUESTION_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2011-11-11/HTMLQuestion.xsd"
	ASSIGNMENT_DURATION      = 600                // How long, in seconds, a worker has to complete the assignment
	LIFETIME                 = 1200 * time.Second // How long, in seconds, before the task expires
	MAX_ASSIGNMENTS          = 1                  // Number of times the task needs to be performed
	AUTO_APPROVAL_DELAY      = 0                  // Seconds before the request is auto-accepted. Set to 0 to accept immediately
	MAX_QUESTION_SIZE        = 65535
	REWARD                   = "0.50"
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

func GetAssignmentsForHITOperation(hitID string) (*GetAssignmentsForHITResponse, error) {
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

	log.Printf("Amazon returned %+v", string(bts))
	var result GetAssignmentsForHITResponse
	err = xml.Unmarshal(bts, &result)
	if err != nil {
		return nil, err
	}
	if !result.GetAssignmentsForHITResult.Request.IsValid {
		return &result, fmt.Errorf("Request invalid or unmarshalling failed")
	}
	return &result, err
}

// Create an HIT
func CreateHIT(title string, description string, htmlQuestionContent HTMLQuestionContent, rewardAmount string, rewardCurrencyCode string, assignmentDuration int, lifetime time.Duration, keywords []string, autoApprovalDelay int, requesterAnnotation string, uniqueRequestToken string, responseGroup string) (*CreateHITResponse, error) {
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
	v.Set("Description", description)
	v.Set("Timestamp", timestamp)
	v.Set("Title", title)
	v.Set("Question", string(bts))
	v.Set("LifetimeInSeconds", strconv.Itoa(int(lifetime.Seconds())))
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

	if !result.HIT.Request.IsValid {
		err = fmt.Errorf("CreateHITResponse contained an invalid response")
	}

	return &result, err
}

func SetHITTypeNotification(hitTypeId string, notification Notification) (*SearchHITsResponse, error) {
	const OPERATION = "SetHITTypeNotification"
	const EVENT_TYPE = "AssignmentSubmitted"
	return nil, nil
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

func CreateTranslationHIT(a *anaconda.TwitterApi, tweet anaconda.Tweet, title string, description string, displayName string, rewardAmount string, assignmentDuration int, lifetime time.Duration, keywords []string) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const service = "AWSMechanicalTurkRequester"
	const operation = "CreateHIT"
	const rewardCurrencyCode = "USD" // This is the only one supported for now by Amazon, anyway
	const responseGroup = "Minimal"
	const autoApprovalDelay = 0 // auto-approve immediately

	log.Print("About to request tweet")

	embed, err := a.GetOEmbedId(tweet.Id, nil)
	if err != nil {
		return nil, err
	}

	log.Print("Successfully obtained embedded tweet")

	hq := HTMLQuestionContent{tweet.Id_str, title, description, "http://www.emojidick.com/emoji.png", tweet, embed}

	resp, err := CreateHIT(title, description, hq, rewardAmount, rewardCurrencyCode, assignmentDuration, lifetime, keywords, autoApprovalDelay, tweet.Id_str, tweet.Id_str+time.Now().String(), responseGroup)
	return resp, err
}

func ScheduleTranslatedTweet(tweet anaconda.Tweet) {

	title := `Translate tweet into emoji`
	description := `Pick the emoji that you feel would be the best translation of this tweet.`
	displayName := "How would you translate this tweet?"
	hit, err := CreateTranslationHIT(twitterBot, tweet, title, description, displayName, REWARD, ASSIGNMENT_DURATION, LIFETIME, HIT_KEYWORDS)
	if err != nil {
		log.Printf("ERROR creating translation HIT: %s", err)
	}

	// Check every minute for the completed task
	hitId := hit.HIT.HITId
	ticker := time.NewTicker(time.Minute)
	timeout := time.After(LIFETIME)
	for {
		log.Printf("Re-entering for loop")
		select {
		case <-ticker.C:
			log.Printf("Fetching assignments for HITOperation")
			result, err := GetAssignmentsForHITOperation(hitId)
			if err != nil {
				log.Printf("ERROR fetching assignments for HITOperation %s: %s", hitId, err)
			}
			answerText, err := result.GetAnswerText()
			if err != nil {
				log.Printf("ERROR getting text of response for HITOperation %s: %s", hitId, err)
			}
			if answerText == "" {
				log.Printf("Assignments yielded an empty response")
				continue
			}
			log.Printf("Received answerText %s", answerText)
			v := url.Values{}
			v.Set("in_reply_to_status_id", tweet.Id_str)
			_, err = twitterBot.PostTweet(fmt.Sprintf("%s %s", *tweet.User.Screen_name, answerText), v)
			if err != nil {
				log.Printf("ERROR updating tweet %s: %s", tweet.Id_str, err)
			}

		case <-timeout:
			log.Printf("Timing out :(")
			return
		}
	}
}

func main() {

	if tmp, err := aws.EnvAuth(); err != nil {
		panic(err)
	} else {
		awsAuth = &tmp
	}

	anaconda.SetConsumerKey(string(TWITTER_CONSUMER_KEY))
	anaconda.SetConsumerSecret(string(TWITTER_CONSUMER_SECRET))
	twitterBot = anaconda.NewTwitterApi(string(TWITTER_ACCESS_TOKEN), string(TWITTER_ACCESS_TOKEN_SECRET))

	me, err := twitterBot.GetSelf(nil)
	if err != nil {
		panic(err)
	}
	log.Printf("My Twitter userId is  %f", *me.Id)

	for {
		tweets, _ := twitterBot.GetHomeTimeline()
		for _, tweet := range tweets {
			// Don't reply to own tweets
			// Only reply to tweets within the last 23 hours
			// Amazon guarantees idempotency of requests with the same unique identifier for 24 hours
			if t, _ := tweet.CreatedAtTime(); time.Now().Add(-23*time.Hour).Before(t) && *tweet.User.Id != *me.Id {
				log.Printf("Scheduling response to tweet %s", tweet.Text)
				go ScheduleTranslatedTweet(tweet)
			}
			log.Printf("Ignoring tweet %s", tweet.Text)
		}
		// TODO fix pagination
		log.Printf("Finished scanning all tweets")
		<-time.After(10 * time.Minute)
	}
}
