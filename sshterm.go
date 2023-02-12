package sshterm

import (
	"errors"
	"net"
	"os"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

type sshTerm struct {
	session *ssh.Session
	client  *ssh.Client
}

type Config struct {
	User          string
	Host          string
	KnownHostPath string
}

func NewSSHTerm(config Config) (sshTerm, error) {
	st := sshTerm{}
	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return st, errors.New("cannot connect to SSH sock: \n" + err.Error())
	}

	agent := agent.NewClient(sock)
	signers, err := agent.Signers()
	if err != nil {
		return st, errors.New("cannot create signers: \n" + err.Error())
	}

	hostKey, err := knownhosts.New(config.KnownHostPath)
	if err != nil {
		return st, errors.New("unable to parse known_hosts: \n" + err.Error())
	}

	clientConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signers...)},
		HostKeyCallback: hostKey,
	}

	client, err := ssh.Dial("tcp", config.Host+":22", clientConfig)
	if err != nil {
		return st, errors.New("unable to connect: \n" + err.Error())
	}

	st.client = client

	session, err := client.NewSession()
	if err != nil {
		return st, errors.New("unable to create session: \n" + err.Error())
	}
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	st.session = session

	return st, nil
}

func (s *sshTerm) Close() {
	s.session.Close()
	s.client.Close()
}

func (s *sshTerm) Terminal(cmd ...string) error {
	var err error

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,     // enable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}

	fileDescriptor := int(os.Stdin.Fd())

	if term.IsTerminal(fileDescriptor) {
		originalState, err := term.MakeRaw(fileDescriptor)
		if err != nil {
			return err
		}
		defer term.Restore(fileDescriptor, originalState)

		termWidth, termHeight, err := term.GetSize(fileDescriptor)
		if err != nil {
			return err
		}

		// Если создать консоль в ITerm2, а потом создать новую вкладку,
		// то у нас одна строка отрежется (так как добавится строка с вкладками).
		// Из-за этого некорректно будет отображаться текст.
		// Для решения проблемы создается терминал заранее на одну строку меньше.
		termHeight -= 1

		err = s.session.RequestPty("xterm-256color", termHeight, termWidth, modes)
		if err != nil {
			return err
		}
	}

	if len(cmd) > 0 {
		err = s.session.Start(cmd[0])
	} else {
		err = s.session.Shell()
	}

	if err != nil {
		return err
	}

	s.session.Wait()

	return nil
}

func (s *sshTerm) Exec(cmd string) error {
	return s.session.Run(cmd)
}
