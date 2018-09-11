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
			message += "\nHost: " + conf.Host + ":" + strconv.Itoa(conf.Port)
		}

		fmt.Println(message)
	}

	for _, config := range confs {
		conn, err := BuildConn(&config)

		if err != nil {
			err := fmt.Errorf("Invalid connection %#v: %v", config, err)
			ch <- &result{nil, err}
			return err
		}

		connsWaitingFor = append(connsWaitingFor, conn)

		go func(conf Config) {
			ch <- DialConn(conn, &conf, print)
		}(config)
	}

	for i := 0; i < len(confs); i++ {
		res := <-ch

		if waitingFor {
			connsWaitingFor = removeConn(connsWaitingFor, res.conn)
			if len(connsWaitingFor) > 0 {
				message := "\033[H\033[2JWaiting for: "

				for _, conn := range connsWaitingFor {
					message += "\nHost: " + conn.URL.Host
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
func DialConn(conn *Connection, conf *Config, print func(a ...interface{})) *result {
	print("Waiting " + strconv.Itoa(conf.Timeout) + " seconds")
	res := pingHost(conn, conf, print)

	if res.err != nil {
		return res
	}

	if conn.URL.Scheme == "http" || conn.URL.Scheme == "https" {
		return pingAddress(conn, conf, print)
	}

	return res
}

// pingAddress check if the full address is responding properly
func pingAddress(conn *Connection, conf *Config, print func(a ...interface{})) *result {
	timeout := time.Duration(conf.Timeout) * time.Second
	start := time.Now()
	address := conn.URL.String()
	print("Ping http address: " + address)
	res := &result{
		conn: conn,
		err:  nil,
	}
	if conf.Status > 0 {
		print("Expect HTTP status" + strconv.Itoa(conf.Status))
	}

	client := &http.Client{}
	req, err := http.NewRequest("GET", address, nil)
	if err != nil {
		res.err = fmt.Errorf("Error creating request: %v", err)
		return res
	}

	for k, v := range conf.Headers {
		print("Adding header " + k + ": " + v)
		req.Header.Add(k, v)
	}

	for {
		resp, err := client.Do(req)
		if resp != nil {
			print("Ping http address " + address + " " + resp.Status)
		}

		if err == nil {
			if conf.Status > 0 && conf.Status == resp.StatusCode {
				return res
			} else if conf.Status == 0 && resp.StatusCode < http.StatusInternalServerError {
				return res
			}
		}

		if time.Since(start) > timeout {
			res.err = errors.New(resp.Status)
			return res
		}

		time.Sleep(time.Duration(conf.Retry) * time.Millisecond)
	}
}

// pingHost check if the host (hostname:port) is responding properly
func pingHost(conn *Connection, conf *Config, print func(a ...interface{})) *result {
	timeout := time.Duration(conf.Timeout) * time.Second
	start := time.Now()
	address := conn.URL.Host
	print("Ping host: " + address)
	res := &result{
		conn: conn,
		err:  nil,
	}

	for {
		_, err := net.DialTimeout(conn.NetworkType, address, time.Second)
		print("Ping host: " + address)

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

		time.Sleep(time.Duration(conf.Retry) * time.Millisecond)
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
