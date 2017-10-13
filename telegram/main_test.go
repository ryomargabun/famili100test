package main

import (
	"fmt"
	l "log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/uber-go/zap"
	"github.com/yulrizka/bot"
	"github.com/yulrizka/fam100"
)

func TestMain(m *testing.M) {
	if _, err := fam100.InitQuestion("../test.db"); err != nil {
		panic(err)
	}
	if err := fam100.DefaultDB.Init(); err != nil {
		panic(err)
	}
	fam100.DefaultDB.Reset()
	retCode := m.Run()
	fam100.DefaultQuestionDB.Close()
	os.Exit(retCode)
}

func TestQuorumShouldStartGame(t *testing.T) {
	oMinQuorum := minQuorum
	defer func() {
		minQuorum = oMinQuorum
	}()
	minQuorum = 2
	log.SetLevel(zap.ErrorLevel)
	fam100.SetLogger(log)
	// create a new game
	out := make(chan bot.Message)
	b := fam100Bot{}
	in, err := b.Init(out)
	if err != nil {
		t.Error(err)
	}

	chanID := "1"
	player1 := bot.User{ID: "ID1", FirstName: "Player 1"}
	player2 := bot.User{ID: "ID2", FirstName: "Player 2"}
	players := []bot.User{player1, player2}
	b.start()

	// send join message 3 time from the same person
	msg := bot.Message{
		From: player1,
		Chat: bot.Chat{ID: chanID, Type: bot.Group},
		Text: "/join@" + botName,
	}
	for i := 0; i < 3; i++ {
		in <- &msg
	}

	reply := readOutMessage(t, &b)
	if _, ok := reply.(bot.Message); !ok {
		t.Fatalf("expecting message got %v", reply)
	}
	g, ok := b.channels[chanID]
	if !ok {
		t.Fatalf("failed to get channel")
	}
	if want, got := 1, len(g.quorumPlayer); want != got {
		t.Fatalf("quorum want %d, got %d", want, got)
	}
	if want, got := fam100.Created, g.game.State; want != got {
		t.Fatalf("state want %s, got %s", want, got)
	}

	// message to another channel, should not affect the state
	in <- &bot.Message{
		From: player2,
		Chat: bot.Chat{ID: "2", Type: bot.Group},
		Text: "/join@" + botName,
	}
	reply = readOutMessage(t, &b)
	if _, ok := reply.(bot.Message); !ok {
		t.Fatalf("expecting message got %v", reply)
	}
	if want, got := fam100.Created, g.game.State; want != got {
		t.Fatalf("state want %s, got %s", want, got)
	}
	if want, got := 1, len(g.quorumPlayer); want != got {
		t.Fatalf("quorum want %d, got %d", want, got)
	}

	// message with quorum should start the game
	in <- &bot.Message{
		From: bot.User{ID: "4", FirstName: "Foo"},
		Chat: bot.Chat{ID: chanID, Type: bot.Group},
		Text: "/join@" + botName,
	}

	// notification game is started
	reply = readOutMessage(t, &b)
	if _, ok := reply.(bot.Message); !ok {
		t.Fatalf("expecting message got %v", reply)
	}
	if want, got := fam100.Started, g.game.State; want != got {
		t.Fatalf("state want %s, got %s", want, got)
	}
	if want, got := minQuorum, len(g.quorumPlayer); want != got {
		t.Fatalf("quorum want %d, got %d", want, got)
	}

	fam100.DelayBetweenRound = 0

	for i := 1; i <= fam100.RoundPerGame; i++ {
		// question
		reply = readOutMessage(t, &b)
		if _, ok := reply.(bot.Message); !ok {
			t.Fatalf("expecting message got %v", reply)
		}

		question := g.game.CurrentQuestion()
		for _, ans := range question.Answers {
			in <- &bot.Message{
				From: players[rand.Intn(len(players))],
				Chat: bot.Chat{ID: chanID, Type: bot.Group},
				Text: ans.Text[0],
			}

			// question with score
			reply = readOutMessage(t, &b)
			if _, ok := reply.(bot.Message); !ok {
				t.Fatalf("expecting message got %v", reply)
			}
		}

		// ranking
		reply = readOutMessage(t, &b)
		if _, ok := reply.(bot.Message); !ok {
			t.Fatalf("expecting message got %v", reply)
		}
	}
	reply = readOutMessage(t, &b)
	if _, ok := reply.(bot.Message); !ok {
		t.Fatalf("expecting message got %v", reply)
	}

	// Game selesai
	if want, got := fam100.Finished, g.game.State; want != got {
		t.Fatalf("state want %s, got %s", want, got)
	}
}

func readOutMessage(t *testing.T, b *fam100Bot) fam100.Message {
	for {
		select {
		case m := <-b.out:
			return m
		case <-time.After(1 * time.Second):
			t.Fatal(fmt.Errorf("timeout waiting to message"))
			return nil
		}
	}
}

func TestGame(t *testing.T) {
	t.Skip()

	out := make(chan bot.Message)
	subject := fam100Bot{}
	in, err := subject.Init(out)
	if err != nil {
		t.Error(err)
	}
	subject.start()

	// read output
	go func() {
		for {
			<-subject.out
		}
		//time.Sleep(70 * time.Millisecond)
	}()

	players := make([]bot.User, 5)
	for i := 0; i < len(players); i++ {
		players[i] = bot.User{ID: fmt.Sprintf("%d", i), FirstName: fmt.Sprintf("Player %d", i)}
	}

	play := func(chatID string) {
		// send join message from 3 different user
		for i := 0; i < 3; i++ {
			in <- &bot.Message{
				From:       players[i],
				Chat:       bot.Chat{ID: chatID, Type: bot.Group},
				Text:       "/join@" + botName,
				ReceivedAt: time.Now(),
			}
		}

		// randomly answer message at a rate
		for {
			select {
			default:
				in <- &bot.Message{
					From:       players[rand.Intn(len(players))],
					Chat:       bot.Chat{ID: chatID, Type: bot.Group},
					Text:       "some answer",
					ReceivedAt: time.Now(),
				}
				in <- &bot.Message{
					From: players[rand.Intn(len(players))],
					Chat: bot.Chat{ID: chatID, Type: bot.Group},
					//Text:       "foo",
					Text:       "/join@" + botName,
					ReceivedAt: time.Now(),
				}
			}
		}
	}

	log.SetLevel(zap.NoneLevel)
	fam100.SetLogger(log)
	for i := 0; i < 500; i++ {
		go play(fmt.Sprintf("%d", i))
	}

	time.Sleep(5 * time.Second)

	go metrics.LogScaled(metrics.DefaultRegistry, 1*time.Second, time.Millisecond, l.New(os.Stderr, "", 0))
	time.Sleep(1*time.Second + 100*time.Millisecond)
}
