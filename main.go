package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/slack-go/slack/slackevents"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/robfig/cron/v3"
	"github.com/slack-go/slack"
)

var signingSecret = os.Getenv("SLACK_SIGNING_SECRET")
var api = slack.New(os.Getenv("SLACK_ACCESS_TOKEN"))

func main() {
	users, err := api.GetUsers()
	if err != nil {
		fmt.Println(users)
		log.Fatalf("Error getting users: %s", err)
	}

	c := cron.New()

	_, err = c.AddFunc("0 10 * * *", func() {
		for _, user := range users {
			if user.IsBot || user.Deleted {
				continue
			}
			sendMorningGreetingMessageToUser(user)
		}
	})
	if err != nil {
		log.Fatalf("Error doing cron job: %s", err)
	}

	http.HandleFunc("/events-endpoint", handleEvents)
	http.HandleFunc("/oauth", handleOAuth)
	http.HandleFunc("/index", handleIndex)
	fmt.Println("[INFO] Server listening")
	http.ListenAndServe(":8010", nil)

	c.Start()
}

func sendWelcomeMessageToUser(user slack.User) {
	welcomeMessage := user.RealName + "님 안녕하세요 :)\n" +
		"Kim's Project에 합류하신 것을 환영합니다!\n" +
		"같이 재밌는 서비스를 만들어보아요 XD"
	sendMessageToUser(user, welcomeMessage)
}

func sendMorningGreetingMessageToUser(user slack.User) {
	moriningGreetingMessage := user.RealName + "님 좋은 아침이에요 :)\n" +
		"오늘도 즐겁게 일해봐요 XD"
	sendMessageToUser(user, moriningGreetingMessage)
}

func sendMessageToUser(user slack.User, message string) {
	_, _, err := api.PostMessage(user.ID, slack.MsgOptionText(message, false))
	if err != nil {
		log.Printf("Error sending welcome message to %s: %s", user.RealName, err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("index"))
}

func handleOAuth(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	fmt.Printf("%s\n", code)

	res, err := slack.GetOAuthV2ResponseContext(context.Background(), &http.Client{}, "SLACK_CLIENT_ID", "SLACK_CLIENT_SECRET", code, "REDIRECT_URL")
	fmt.Printf("%v, %v\n", res.AccessToken, err)
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sv, err := slack.NewSecretsVerifier(r.Header, signingSecret)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, err := sv.Write(body); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := sv.Ensure(); err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	eventsAPIEvent, err := slackevents.ParseEvent(json.RawMessage(body), slackevents.OptionNoVerifyToken())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	fmt.Printf("[Event] %v(%v)\n", eventsAPIEvent.Type, eventsAPIEvent.InnerEvent.Type)

	if eventsAPIEvent.Type == slackevents.URLVerification {
		var r *slackevents.ChallengeResponse
		err := json.Unmarshal([]byte(body), &r)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text")
		w.Write([]byte(r.Challenge))
	}
	if eventsAPIEvent.Type == slackevents.CallbackEvent {
		innerEvent := eventsAPIEvent.InnerEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			api.PostMessage(ev.Channel, slack.MsgOptionText("Yes, hello.", false))
		case *slackevents.UserProfileChangedEvent:
			messageText := func(statusText string) string {
				if statusText == "" {
					return fmt.Sprintf("User `%s` has cleared the status.", ev.User.Name)
				}
				return fmt.Sprintf("User `%s`'s status has changed to `%s`.", ev.User.Name, ev.User.Profile.StatusText)
			}(ev.User.Profile.StatusText)

			a, b, e := api.PostMessage("playground", slack.MsgOptionText(messageText, false))
			fmt.Printf("%v, %v, %v\n", a, b, e)
		}
	}
}
