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
    "github.com/glycerine/rbtree"
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
    "html/template"
    "strconv"
)

type comment struct {
	ID     int64  `json:"id"`
	Author string `json:"author"`
	Text   string `json:"text"`
}

type treeData struct {
    TimeStamp time.Time
    Index int
}

func treeCompare(a,b rbtree.Item) int {
    switch a.(type) {
    case treeData:
        f := a.(treeData)
        s := b.(treeData)
        if f.TimeStamp.After(s.TimeStamp) {
            return 1
        }else{
            return -1
        }
    default:
        return 0
    }
}

type session struct {
    UserID string
    LastLogIn time.Time
    ShowAll bool
}

var sessions map[int]session

const dataFile = "./comments.json"
var sessionMutex =new(sync.Mutex)
var sessionID int = 0
var messageMutex = new(sync.Mutex)

var timestree *rbtree.Tree
var messages []comment

var db *sql.DB

func getSince(t time.Time) []comment {
    //fmt.Println("I think the error is caused belwo")
    iter := timestree.FindGE(treeData{TimeStamp: t})
    if iter.Limit(){
        return make([]comment,0)
    }
    index := iter.Item()
    //fmt.Println("error happened yet?")
    switch index.(type) {
    case treeData:
        i := index.(treeData)
        return messages[i.Index:]
    default:
        return messages
    }
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
    cookie, _ := r.Cookie("sessionID")
    if cookie != nil {
        sid,_ := strconv.Atoi(cookie.Value)
        _,ok := sessions[sid]
        if ok {
            http.Redirect(w,r,"/messages",http.StatusFound)
        }
    }
    switch r.Method {
    case "POST":
        r.ParseForm()
        user := r.FormValue("username")
        password := r.FormValue("password")
        stmt,_ := db.Prepare("SELECT * FROM logins WHERE username=?")
        rows,err := stmt.Query(user)
        if err != nil {
            fmt.Print(err.Error())
        }
        if rows.Next(){
            var dbu string
            var dbpass string
            var lastlogin time.Time
            err = rows.Scan(&dbu,&dbpass,&lastlogin)
            if err != nil {
                fmt.Println(err.Error())
            }
            if password == dbpass {
                newSession := session{UserID :user, LastLogIn: lastlogin,ShowAll: false}
                sessionMutex.Lock()
                thisID := sessionID +1
                sessionID = thisID
                sessions[sessionID] = newSession
                sessionMutex.Unlock()
                stmt,_ = db.Prepare("UPDATE logins SET lastlogin=? WHERE username=?")
                res,err := stmt.Exec(time.Now(),user)
                fmt.Println(res.RowsAffected())
                if err != nil {
                    fmt.Println(err.Error())
                }
                expiration := time.Now().Add(24 * time.Hour)
                cookie := http.Cookie{Name: "sessionID", Value: strconv.Itoa(thisID), Expires: expiration}
                http.SetCookie(w,&cookie)
                http.Redirect(w,r,"/messages",http.StatusFound)
            }else{
                fmt.Println("wrong password")
                http.Redirect(w,r,"/login",http.StatusFound)
            }
        }else{
            stmt,_ = db.Prepare("INSERT logins SET username=?,password=?,lastlogin=?")
            _,err = stmt.Query(user,password,time.Now())
            newSession := session{UserID: user, LastLogIn: time.Now(), ShowAll: false}
            sessionMutex.Lock()
            thisID := sessionID+1
            sessionID = thisID
            sessions[thisID] = newSession
            sessionMutex.Unlock()
            expiration := time.Now().Add(24 * time.Hour)
            cookie := http.Cookie{Name: "sessionID", Value: strconv.Itoa(thisID), Expires: expiration}
            http.SetCookie(w,&cookie)
            http.Redirect(w,r,"/messages",http.StatusFound)
        }
    case "GET":
        t,_ := template.ParseFiles("public/login.html")
        t.Execute(w,nil)
    default:
		http.Error(w, fmt.Sprintf("Unsupported method: %s", r.Method), http.StatusMethodNotAllowed)
    }
}

// Handle comments
func handleComments(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("sessionID")
    if err != nil {
        //fmt.Println(err.Error())
        return
    }
    currID,_ := strconv.Atoi(cookie.Value)
    currSession := sessions[currID]
    //fmt.Println(cookie)
	switch r.Method {
	case "POST":
        //fmt.Println("There was a new message sent")
		// Add a new comment to the in memory slice of comments
        author := currSession.UserID
        text := r.FormValue("text")
        if text == "" {
            //fmt.Println("changed select?")
            timeframe := r.FormValue("select")
            //fmt.Println(timeframe)
            if timeframe == "New" {
                currSession.ShowAll = false
                sessionMutex.Lock()
                sessions[currID] = currSession
                sessionMutex.Unlock()
                fmt.Println(currSession.LastLogIn)
            }else {
                currSession.ShowAll = true
                sessionMutex.Lock()
                sessions[currID] = currSession
                sessionMutex.Unlock()
            }
        }else{
        id := time.Now()
        messageMutex.Lock()
        timestree.Insert(treeData{TimeStamp: id, Index: len(messages)})
		messages = append(messages, comment{ID: (id.UnixNano()/1000000), Author: author, Text: text})
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
        }
		// Marshal the comments to indented json.
		var marshMessages []comment
        if currSession.ShowAll {
            marshMessages = messages
        }else{
            marshMessages = getSince(currSession.LastLogIn)
        }
		messageData, err := json.MarshalIndent(marshMessages, "", "    ")
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to marshal comments to json: %s", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		io.Copy(w, bytes.NewReader(messageData))

	case "GET":
        var marshMessages []comment
        if currSession.ShowAll {
            marshMessages = messages
        }else{
            marshMessages = getSince(currSession.LastLogIn)
        }
        messageData, err := json.MarshalIndent(marshMessages, "", "    ")
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

func redirectHandle(w http.ResponseWriter, r *http.Request) {
    http.Redirect(w,r,"/login",http.StatusFound)
}

func messagesHandle(w http.ResponseWriter, r *http.Request) {
    cookie, err := r.Cookie("sessionID")
    if err != nil {
        http.Redirect(w,r,"/login",http.StatusFound)
    }
    if cookie != nil {
        sid,_ := strconv.Atoi(cookie.Value)
        _,ok := sessions[sid]
        if !ok {
            http.Redirect(w,r,"/login",http.StatusFound)
        }else{
            //fmt.Println("was redirected to messages")
            http.FileServer(http.Dir("./public"))
            http.ServeFile(w,r,"./public/messages.html")
        }
    }
}

func main() {
    sessions = make(map[int]session)
    var err error
    db, err = sql.Open("mysql", "root:angel@/test?charset=utf8&parseTime=true&loc=Local")
    if err != nil{
        fmt.Println(err.Error())
        return
    }

    err = db.Ping()
    if err != nil {
        fmt.Println(err.Error())
        return
    }
    _, err = db.Exec("CREATE TABLE IF NOT EXISTS `messages` ("+
                "`timestamp` DATETIME DEFAULT CURRENT_TIMESTAMP,"+
                "`user` VARCHAR(64) NULL DEFAULT NULL,"+
                "`message` VARCHAR(200) NULL DEFAULT NULL,"+
                "PRIMARY KEY (`timestamp`)"+
              ");")
    if err != nil {
        fmt.Println(err.Error())
        return
    }
    _, err = db.Exec("CREATE TABLE IF NOT EXISTS `logins` ("+
                    "`username` VARCHAR(64) NOT NULL,"+
                    "`password` VARCHAR(20) NULL DEFAULT NULL,"+
                    "`lastlogin` DATETIME DEFAULT CURRENT_TIMESTAMP,"+
                    "PRIMARY KEY (`username`)"+
                ");")
    if err != nil {
        fmt.Println(err.Error())
        return
    }

    rows, errdb := db.Query("SELECT * FROM messages ORDER BY timestamp ASC;")
    if errdb != nil {
        fmt.Println("errdb.Error()")
        return
    }
    timestree = rbtree.NewTree(treeCompare)
    for i:=0; rows.Next(); {
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
        timeValue := treeData { TimeStamp: timestamp, Index: i}
        timestree.Insert(timeValue)
        //fmt.Printf("mid:%d, user:%s, message:%s, time:%s\n",mid,user,text,timestamp)
    }
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/api/comments", handleComments)
	http.HandleFunc("/messages", messagesHandle)
    http.HandleFunc("/login", handleLogin)
    http.HandleFunc("/",redirectHandle)
	log.Println("Server started: http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
