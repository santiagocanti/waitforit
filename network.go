package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"
)

type result struct {
	conn *Connection
	err  error
}

// DialConfigs dial multiple connections at same time
func DialConfigs(confs []Config, print func(a ...interface{}), waitingFor bool) error {
	ch := make(chan *result)
	var connsWaitingFor []*Connection

	if waitingFor {
		message := "\033[H\033[2JWaiting for: "

		for _, conf := range confs {
			message += "\nHost: " + conf.Host + " Port: " + strconv.Itoa(conf.Port)
		}

		fmt.Println(message)
	}

	for _, config := range confs {
		conn := BuildConn(&config)

		if conn == nil {
			err := fmt.Errorf("Invalid connection %#v", config)
			ch <- &result{nil, err}
			return err
		}

		connsWaitingFor = append(connsWaitingFor, conn)

		go func(conf Config) {
			ch <- DialConn(conn, conf.Timeout, conf.Retry, print)
		}(config)
	}

	for i := 0; i < len(confs); i++ {
		res := <-ch

		if waitingFor {
			connsWaitingFor = removeConn(connsWaitingFor, res.conn)
			if len(connsWaitingFor) > 0 {
				message := "\033[H\033[2JWaiting for: "

				for _, conn := range connsWaitingFor {
					message += "\nHost: " + conn.Host + " Port: " + strconv.Itoa(conn.Port)
				}

				fmt.Println(message)
			} else {
				fmt.Println("\033[H\033[2JAll hosts are running")
			}
		}

		if res.err != nil {
			return res.err
		}
	}

	return nil
}

// DialConn check if the connection is available
func DialConn(conn *Connection, timeoutSeconds int, retryMseconds int, print func(a ...interface{})) *result {
	print("Waiting " + strconv.Itoa(timeoutSeconds) + " seconds")
	res := pingTCP(conn, timeoutSeconds, retryMseconds, print)

	if res.err != nil {
		return res
	}

	if conn.Scheme != "http" && conn.Scheme != "https" {
		return res
	}

	return pingHTTP(conn, timeoutSeconds, retryMseconds, print)
}

func pingHTTP(conn *Connection, timeoutSeconds int, retryMseconds int, print func(a ...interface{})) *result {
	timeout := time.Duration(timeoutSeconds) * time.Second
	start := time.Now()
	address := fmt.Sprintf("%s://%s:%d%s", conn.Scheme, conn.Host, conn.Port, conn.Path)
	print("HTTP address: " + address)
	res := &result{
		conn: conn,
		err:  nil,
	}

	for {
		resp, err := http.Get(address)

		if resp != nil {
			print("ping HTTP " + address + " " + resp.Status)
		}

		if err == nil && resp.StatusCode < http.StatusInternalServerError {
			return res
		}

		if time.Since(start) > timeout {
			res.err = errors.New(resp.Status)
			return res
		}

		time.Sleep(time.Duration(retryMseconds) * time.Millisecond)
	}
}

func pingTCP(conn *Connection, timeoutSeconds int, retryMseconds int, print func(a ...interface{})) *result {
	timeout := time.Duration(timeoutSeconds) * time.Second
	start := time.Now()
	address := fmt.Sprintf("%s:%d", conn.Host, conn.Port)
	print("Dial address: " + address)
	res := &result{
		conn: conn,
		err:  nil,
	}

	for {
		_, err := net.DialTimeout(conn.Type, address, time.Second)
		print("ping TCP: " + address)

		if err == nil {
			print("Up: " + address)
			return res
		}

		print("Down: " + address)
		print(err)
		if time.Since(start) > timeout {
			res.err = err
			return res
		}

		time.Sleep(time.Duration(retryMseconds) * time.Millisecond)
	}
}

func removeConn(connsWaitingFor []*Connection, connToRemove *Connection) []*Connection {
	for i, conn := range connsWaitingFor {
		if conn == connToRemove {
			connsWaitingFor = append(connsWaitingFor[:i], connsWaitingFor[i+1:]...)
			break
		}
	}

	return connsWaitingFor
}
