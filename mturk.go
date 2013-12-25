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
	"strconv"
	"strings"
	"time"
)

const (
	QUESTION_FORM_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2005-10-01/QuestionForm.xsd"
	HTML_QUESTION_SCHEMA_URL = "http://mechanicalturk.amazonaws.com/AWSMechanicalTurkDataSchemas/2011-11-11/HTMLQuestion.xsd"
)

func executePostQuery(auth *aws.Auth, queryUrl, service, operation string, v url.Values, result interface{}) error {
	sign(*awsAuth, service, operation, v)
	resp, err := http.PostForm(queryUrl, v)
	if err != nil {
		return err
	}
	bts, err := ioutil.ReadAll(resp.Body)
	log.Printf("Amazon returned %+v", string(bts))
	if err != nil {
		return err
	}
	return xml.Unmarshal(bts, &result)
}

func sign(auth aws.Auth, service, operation string, v url.Values) {
	timestamp := time.Now().Format(time.RFC3339)
	b64 := base64.StdEncoding
	payload := service + operation + timestamp
	hash := hmac.New(sha1.New, []byte(auth.SecretKey))
	hash.Write([]byte(payload))
	signature := make([]byte, b64.EncodedLen(hash.Size()))
	b64.Encode(signature, hash.Sum(nil))
	v.Set("Signature", string(signature))
	v.Set("Operation", operation)
	v.Set("Version", "2012-03-25")
	v.Set("AWSAccessKeyId", awsAuth.AccessKey)
	v.Set("Timestamp", timestamp)
}

// GetAssignmentsForHITOperation fetches the assignment details for the HIT with id hitID
func GetAssignmentsForHITOperation(auth *aws.Auth, hitID string) (*GetAssignmentsForHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const OPERATION = "GetAssignmentsForHIT"
	const SERVICE = "AWSMechanicalTurkRequester"
	v := url.Values{}
	v.Set("HITId", hitID)
	var result GetAssignmentsForHITResponse
	err := executePostQuery(auth, QUERY_URL, SERVICE, OPERATION, v, &result)
	if err != nil {
		return nil, err
	}
	if !result.GetAssignmentsForHITResult.Request.IsValid {
		return &result, fmt.Errorf("Request invalid or unmarshalling failed")
	}
	return &result, err
}

// Create an HIT
func CreateHIT(auth *aws.Auth, title string, description string, questionContentString, rewardAmount string, rewardCurrencyCode string, assignmentDuration int, lifetime time.Duration, keywords []string, autoApprovalDelay int, requesterAnnotation string, uniqueRequestToken string, responseGroup string) (*CreateHITResponse, error) {
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const SERVICE = "AWSMechanicalTurkRequester"
	const OPERATION = "CreateHIT"

	v := url.Values{}
	v.Set("Description", description)
	v.Set("Title", title)
	v.Set("Question", questionContentString)
	v.Set("LifetimeInSeconds", strconv.Itoa(int(lifetime.Seconds())))
	v.Set("Reward.1.Amount", rewardAmount)
	v.Set("Reward.1.CurrencyCode", rewardCurrencyCode)
	v.Set("AssignmentDurationInSeconds", strconv.Itoa(assignmentDuration))
	v.Set("Keywords", strings.Join(keywords, ","))
	v.Set("AutoApprovalDelayInSeconds", strconv.Itoa(autoApprovalDelay))
	v.Set("RequesterAnnotation", requesterAnnotation)
	v.Set("UniqueRequestToken", uniqueRequestToken)
	v.Set("ResponseGroup", responseGroup)

	var result CreateHITResponse
	err := executePostQuery(auth, QUERY_URL, SERVICE, OPERATION, v, &result)
	if err != nil {
		return nil, err
	}
	if !result.HIT.Request.IsValid {
		err = fmt.Errorf("CreateHITResponse contained an invalid response")
	}
	return &result, err
}

func SearchHIT(v url.Values) (*SearchHITsResponse, error) {
	const OPERATION = "SearchHITs"
	const QUERY_URL = "https://mechanicalturk.amazonaws.com/?Service=AWSMechanicalTurkRequester"
	const SERVICE = "AWSMechanicalTurkRequester"
	if v == nil {
		v = url.Values{}
	}

	sign(*awsAuth, SERVICE, OPERATION, v)

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

type Notification struct {
	XMLName     xml.Name `xml:"Notification"`
	Destination string
	Transport   string
	Version     string
	EventType   string
}

type SetHITTypeNotificationResponse struct {
	XMLName xml.Name `xml:"SetHITTypeNotificationResponse"`
	Request struct {
		IsValid bool
	}
}

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

type ReceiveMessageResponse struct {
	XMLName              xml.Name `xml:"ReceiveMessageResponse"`
	ReceiveMessageResult struct {
		Message struct {
			MessageId     string
			ReceiptHandle string
			MD5OfBody     string
			Body          string
			Attribute     []struct {
				Name  string
				Value string
			}
		}
	}
	ResponseMetadata struct {
		RequestId string
	}
}

type GetAssignmentsForHITResponse struct {
	XMLName                    xml.Name `xml:"GetAssignmentsForHITResponse"`
	OperationRequest           string
	RequestId                  string
	GetAssignmentsForHITResult GetAssignmentsForHITResult
}

// GetAnswer will unmarshal the string Answer into a native Go struct
// (This cannot be done at the same time that the parent struct is unmarshaled)
func (r GetAssignmentsForHITResponse) GetAnswer() (*QuestionFormAnswers, error) {
	tmp := r.GetAssignmentsForHITResult.Assignment.Answer
	var result QuestionFormAnswers
	err := xml.Unmarshal([]byte(tmp), &result)
	return &result, err
}

func (r GetAssignmentsForHITResponse) GetAnswerText() (result string, err error) {
	qfa, err := r.GetAnswer()
	if err != nil {
		return
	}
	result = qfa.Answer.FreeText
	return
}

type GetAssignmentsForHITResult struct {
	XMLName xml.Name `xml:"GetAssignmentsForHITResult"`
	Request struct {
		IsValid bool
	}
	NumResults      int
	TotalNumResults int
	PageNumber      int
	Assignment      struct {
		AssignmentId     string
		WorkerId         string
		HITId            string
		AssignmentStatus string
		AutoApprovalTime string
		AcceptTime       string
		SubmitTime       string
		ApprovalTime     string
		Answer           string
	}
}

type QuestionFormAnswers struct {
	XMLName xml.Name `xml:"QuestionFormAnswers"`
	Xmlns   string   `xml:"xmlns,attr"`
	Answer  struct {
		QuestionIdentifier string
		FreeText           string
	}
}
