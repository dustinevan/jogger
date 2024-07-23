package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
)

// This copied from a previous project
var service string

func init() {
	flag.StringVar(&service, "service", "", "filter which service to see")
}

func main() {
	flag.Parse()
	var b strings.Builder

	// Scan standard input for log data per line.
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024*1024)
	for scanner.Scan() {
		s := scanner.Text()

		// Convert the JSON to a map for processing.
		m := make(map[string]any)
		err := json.Unmarshal([]byte(s), &m)
		if err != nil {
			if service == "" {
				fmt.Println(s)
			}
			continue
		}

		if msg, ok := m["msg"].(string); ok {
			var webctx map[string]interface{}
			if werr := json.Unmarshal([]byte(msg), &webctx); werr == nil {

				// If there's a panic error, convert it to an array of strings, and remove the tabs
				if panicErr, ok2 := webctx["panicError"].(string); ok2 {
					panicErr = strings.ReplaceAll(panicErr, "\t", "")
					errs := strings.Split(panicErr, "\n")
					webctx["panicError"] = errs
				}

				bytes, merr := json.MarshalIndent(webctx, "", "  ")
				if merr == nil {
					m["msg"] = string(append([]byte{'\n'}, bytes...))
				}
			}
		}

		// If a service filter was provided, check.
		if service != "" && m["service"] != service {
			continue
		}

		// Build out the know portions of the log in the order
		// I want them in.
		b.Reset()
		b.WriteString(fmt.Sprintf("--------------------------------------------------\n%s: %s: %s: %s: %s: ",
			m["service"],
			m["ts"],
			m["level"],
			m["caller"],
			m["msg"],
		))

		// Add the rest of the keys ignoring the ones we already
		// added for the log.
		var customFields []string
		for k, v := range m {
			switch k {
			case "service", "ts", "level", "caller", "msg":
				continue
			}
			customFields = append(customFields, fmt.Sprintf("%s[%v]: ", k, v))
		}
		sort.Strings(customFields)
		b.WriteString(strings.Join(customFields, ""))

		// Write the new log format, removing the last :
		out := b.String()
		fmt.Println(out[:len(out)-2])
	}

	if err := scanner.Err(); err != nil {
		log.Println(err)
	}
}
