package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"github.com/ChimeraCoder/anaconda"
	"io/ioutil"
)

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
		Answer           QuestionFormAnswers
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
