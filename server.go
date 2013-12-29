package main

import (
	"bytes"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"github.com/ChimeraCoder/mtwerk"
	"io/ioutil"
	"launchpad.net/goamz/aws"
	"log"
	"net/url"
	"os"
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
	ASSIGNMENT_DURATION = 600                // How long, in seconds, a worker has to complete the assignment
	LIFETIME            = 1200 * time.Second // How long, in seconds, before the task expires
	MAX_ASSIGNMENTS     = 1                  // Number of times the task needs to be performed
	AUTO_APPROVAL_DELAY = 0                  // Seconds before the request is auto-accepted. Set to 0 to accept immediately
	MAX_QUESTION_SIZE   = 65535
	REWARD              = "0.50"
)

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

func parseQuestionContent(htmlQuestionContent mtwerk.HTMLQuestionContent) (result string, err error) {
	// TODO move this elsewhere
	bs := make([]byte, 0, MAX_QUESTION_SIZE)
	bf := bytes.NewBuffer(bs)
	tmpl, err := template.New("foo").Parse(HTMLQuestionTemplate)
	if err != nil {
		return
	}
	err = tmpl.ExecuteTemplate(bf, "T", htmlQuestionContent)
	if err != nil {
		return
	}
	bts, err := ioutil.ReadAll(bf)
	if err != nil {
		return
	}
	result = string(bts)
	return
}

func CreateTranslationHIT(a *anaconda.TwitterApi, auth *aws.Auth, tweet anaconda.Tweet, title string, description string, displayName string, rewardAmount string, assignmentDuration int, lifetime time.Duration, keywords []string) (*mtwerk.CreateHITResponse, error) {
	const rewardCurrencyCode = "USD" // This is the only one supported for now by Amazon, anyway
	const responseGroup = "Minimal"
	const autoApprovalDelay = 0 // auto-approve immediately

	log.Print("About to request tweet")

	embed, err := a.GetOEmbedId(tweet.Id, nil)
	if err != nil {
		return nil, err
	}

	log.Print("Successfully obtained embedded tweet")

	hq := mtwerk.HTMLQuestionContent{tweet.IdStr, title, description, "http://www.emojidick.com/emoji.png", tweet, embed}

	questionString, err := parseQuestionContent(hq)
	if err != nil {
		return nil, err
	}

	resp, err := mtwerk.CreateHIT(auth, title, description, questionString, rewardAmount, rewardCurrencyCode, assignmentDuration, lifetime, keywords, autoApprovalDelay, tweet.IdStr, tweet.IdStr, responseGroup)
	return resp, err
}

func ScheduleTranslatedTweet(tweet anaconda.Tweet) {

	title := `Translate tweet into emoji`
	description := `Pick the emoji that you feel would be the best translation of this tweet.`
	displayName := "How would you translate this tweet?"
	hit, err := CreateTranslationHIT(twitterBot, awsAuth, tweet, title, description, displayName, REWARD, ASSIGNMENT_DURATION, LIFETIME, HIT_KEYWORDS)
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
			result, err := mtwerk.GetAssignmentsForHITOperation(awsAuth, hitId)
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
			v.Set("in_reply_to_status_id", tweet.IdStr)
			_, err = twitterBot.PostTweet(fmt.Sprintf("%s %s", tweet.User.ScreenName, answerText), v)
			if err != nil {
				log.Printf("ERROR updating tweet %s: %s", tweet.IdStr, err)
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
	log.Printf("My Twitter userId is  %d", me.Id)
	log.Printf("My Twitter username is  %d", me.ScreenName)

	for {
		tweets, _ := twitterBot.GetHomeTimeline()
		for _, tweet := range tweets {
			// Don't reply to own tweets
			// Only reply to tweets within the last 23 hours
			// Amazon guarantees idempotency of requests with the same unique identifier for 24 hours
			if t, _ := tweet.CreatedAtTime(); time.Now().Add(-23*time.Hour).Before(t) && tweet.User.Id != me.Id {
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
