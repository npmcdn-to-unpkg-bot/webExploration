/**
 * This file provided by Facebook is for non-commercial testing and evaluation
 * purposes only. Facebook reserves all rights not expressly granted.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
 * FACEBOOK BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN
 * ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
 * WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
 */

package main

import (
    _ "github.com/go-sql-driver/mysql"
    "database/sql"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	//"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

type comment struct {
	ID     int64  `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

const dataFile = "./comments.json"

var messageMutex = new(sync.Mutex)

var messages []comment

var db *sql.DB

func handleLogin(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case "POST":
    case "GET":
    default:
		http.Error(w, fmt.Sprintf("Unsupported method: %s", r.Method), http.StatusMethodNotAllowed)
    }
}

// Handle comments
func handleComments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "POST":
        //fmt.Println("There was a new message sent")
		// Add a new comment to the in memory slice of comments
        author := r.FormValue("author")
        text := r.FormValue("text")
        id := time.Now().UnixNano() / 1000000
        messageMutex.Lock()
		messages = append(messages, comment{ID: id, Author: author, Text: text})
        messageMutex.Unlock()
        stmt, errdb := db.Prepare("INSERT messages SET user=?, message=?")
        if errdb != nil {
            fmt.Println(errdb.Error())
            return
        }
        _, errdb = stmt.Exec(author,text)
        if errdb != nil {
            fmt.Println(errdb.Error())
            return
        }
		// Marshal the comments to indented json.
		messageData, err := json.MarshalIndent(messages, "", "    ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to marshal comments to json: %s", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		io.Copy(w, bytes.NewReader(messageData))

	case "GET":
        messageData, err := json.MarshalIndent(messages, "", "    ")
        if err != nil {
            http.Error(w, fmt.Sprintf("Unable to marshal comments to json: %s", err), http.StatusInternalServerError)
            return
        }
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		// stream the contents of the file to the response
		io.Copy(w, bytes.NewReader(messageData))

	default:
		// Don't know the method, so error
		http.Error(w, fmt.Sprintf("Unsupported method: %s", r.Method), http.StatusMethodNotAllowed)
	}
}

/*func handleLogin(w http.ResponseWriter, r *http.Request) {
    
}*/

func main() {
    var err error
    db, err = sql.Open("mysql", "root:oliver42@/test?charset=utf8&parseTime=true&loc=Local")
    if err != nil{
        fmt.Println(err.Error())
        return
    }

    err = db.Ping()
    if err != nil {
        fmt.Println(err.Error())
        return
    }
    _, err = db.Query("CREATE TABLE IF NOT EXISTS `messages` ("+
                "`timestamp` DATETIME DEFAULT CURRENT_TIMESTAMP,"+
                "`user` VARCHAR(64) NULL DEFAULT NULL,"+
                "`message` VARCHAR(200) NULL DEFAULT NULL,"+
                "PRIMARY KEY (`timestamp`)"+
              ");")
    if err != nil {
        fmt.Println(err.Error())
        return
    }
    rows, errdb := db.Query("SELECT * FROM messages;")
    if errdb != nil {
        fmt.Println("errdb.Error()")
        return
    }
    for rows.Next() {
        var user string
        var text string
        var timestamp time.Time
        err := rows.Scan(&timestamp,&user,&text)
        if err != nil {
            fmt.Println(err.Error())
            return
        }
        message := comment{ID: (timestamp.UnixNano()/1000000), Author: user, Text: text}
        messages = append(messages, message)
        //fmt.Printf("mid:%d, user:%s, message:%s, time:%s\n",mid,user,text,timestamp)
    }
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	http.HandleFunc("/api/comments", handleComments)
	http.Handle("/messages", http.FileServer(http.Dir("./public")))
    http.HandleFunc("/", handleLogin)
	log.Println("Server started: http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
