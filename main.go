package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime/pprof"
	"time"

	"./net/smtp"
)

var (
	myDate     = time.Now()
	myPid      = os.Getpid()
	myhostname = "localhost"
)

var (
	defaultSender    = "from@example.com"
	defaultRecipient = "to@example.com"
	defaultSubject   = "from go-smtp-source"
)

var config *Config

type Config struct {
	Host         string
	Sender       string
	Recipient    string
	MessageCount int
	Sessions     int
	MessageSize  int
	UseTLS       bool
	DontDisc     bool

	tlsConfig *tls.Config
}

func usage(m, def string) string {
	return fmt.Sprintf("%s [default: %s]", m, def)
}

func Parse() error {
	var (
		msgcount  = flag.Int("m", 1, usage("specify a number of messages to send.", "1"))
		msgsize   = flag.Int("l", 0, usage("specify the size of the body.", "0"))
		session   = flag.Int("s", 1, usage("specify a number of cocurrent sessions.", "1"))
		sender    = flag.String("f", defaultSender, usage("specify a sender address.", defaultSender))
		recipient = flag.String("t", defaultRecipient, usage("specify a recipient address.", defaultRecipient))
		usetls    = flag.Bool("tls", false, usage("specify if STARTTLS is needed.", "false"))
		dontdisc  = flag.Bool("d", false, usage("specify if send all messages in one connection.", "false"))
	)

	flag.Parse()

	host := flag.Arg(0)
	if host == "" {
		fmt.Fprintf(os.Stderr, "host is missing\n")
		os.Exit(1)
	}

	config = &Config{
		Host:         host,
		Sender:       *sender,
		Recipient:    *recipient,
		MessageCount: *msgcount,
		MessageSize:  *msgsize,
		Sessions:     *session,
		UseTLS:       *usetls,
		DontDisc:     *dontdisc,

		tlsConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return nil
}

type Client struct {
	c   *smtp.Client
	err error
}

func Dial(addr string) (*Client, error) {
	c, err := smtp.Dial(addr)
	if err != nil {
		return nil, err
	}
	return &Client{
		c: c,
	}, nil
}

func (c *Client) SendMail() error {
	if config.UseTLS {
		if err := c.c.StartTLS(config.tlsConfig); err != nil {
			return err
		}
	} else {
		if err := c.c.Hello(myhostname); err != nil {
			return err
		}
	}

  for i := 0; i < config.MessageCount ; i++ {

		if err := c.c.Mail(config.Sender); err != nil {
			return err
		}
		if err := c.c.Rcpt(config.Recipient); err != nil {
			return err
		}

		wc, err := c.c.Data()
		if err != nil {
			return err
		}

		fmt.Fprintf(wc, "From: <%s>\n", config.Sender)
		fmt.Fprintf(wc, "To: <%s>\n", config.Recipient)
		fmt.Fprintf(wc, "Date: %s\n", myDate.Format(time.RFC1123))
		fmt.Fprintf(wc, "Subject: %s\n", defaultSubject)
		fmt.Fprintf(wc, "Message-Id: <%04x.%04x@%s>\n", myPid, config.MessageCount, myhostname)
		fmt.Fprintln(wc, "")

		if config.MessageSize == 0 {
			for i := 1; i < 5; i++ {
				fmt.Fprintf(wc, "La de da de da %d.\n", i)
			}
		} else {
			for i := 1; i < config.MessageSize; i++ {
				fmt.Fprint(wc, "X")
				if i%80 == 0 {
					fmt.Fprint(wc, "\n")
				}
			}
		}

		if err := wc.Close(); err != nil {
			return err
		}

		if (!config.DontDisc) {
			break
		}

  }

	if err := c.c.Quit(); err != nil {
		return err
	}
	return nil
}

func main() {
	if profile := os.Getenv("CPU_PPROF_FILE"); profile != "" {
		f, err := os.Create(profile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if err := Parse(); err != nil {
		panic(err)
	}

	queue := make(chan *Client)
	done := make(chan struct{}, config.MessageCount)

	Launch(queue, done)

	go Kick(queue, done)

	for i := 0; i < config.MessageCount; i++ {
		<-done
		if (config.DontDisc) {
			break
		}
	}

	if profile := os.Getenv("HEAP_PPROF_FILE"); profile != "" {
		f, err := os.Create(profile)
		if err != nil {
			panic(err)
		}
		pprof.WriteHeapProfile(f)
		f.Close()
	}
}

func Launch(queue chan *Client, done chan struct{}) {
	for i := 0; i < config.Sessions; i++ {
		go Worker(queue, done)
	}
}

func Kick(queue chan *Client, done chan struct{}) {
	var messageCount = config.MessageCount
	if (config.DontDisc) {
		messageCount = 1
	}
	for i := 0; i < messageCount; i++ {
		c, err := Dial(config.Host)
		if err != nil {
			log.Print(err)
			done <- struct{}{}
			continue
		}
		queue <- c
	}
}

func Worker(queue <-chan *Client, done chan<- struct{}) {
	for c := range queue {
		c.SendMail()
		done <- struct{}{}
	}
}
