package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/go-sql-driver/mysql"
) //ref = https://go.dev/wiki/SQLDrivers

var db *sql.DB
var tmpl *template.Template

type Album struct {
	ID      int64
	Title   string
	Artist  string
	Price   float32
	LocalFg bool
}

// --page related--//
func viewHandler(w http.ResponseWriter, r *http.Request) {
	//explanation = this opens automatically when http://localhost:8080/view/ is accessed
	if r.URL.Path == "/view" || r.URL.Path == "/view/" {
		albums, err := albumsByArtist("")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tmpl.Execute(w, albums) //execute means Render the page
		return
	}

	//search with key
	key := strings.TrimPrefix(r.URL.Path, "/view/")
	key, _ = url.PathUnescape(key)

	albums, err := albumsByArtist(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, albums)
}

func saveHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	fmt.Println(path) //cth path = "/edit/1"

	// Handle GET requests - show form or delete
	if r.Method == "GET" {
		if path == "/create/" || path == "/create" {
			http.ServeFile(w, r, "create.html") //link to create page
		} else if len(path) > len("/edit/") && path[:len("/edit/")] == "/edit/" {
			//in previous row, check path length, if "len(path)" is longer, it means there is an ID

			//page id validation
			idStr := path[len("/edit/"):] //get the id, it's after edit
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				http.Error(w, "Invalid album ID", http.StatusBadRequest)
				return
			}

			album, err := albumByID(id)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			t, _ := template.ParseFiles("edit.html")
			t.Execute(w, album)
		} else if len(path) > len("/delete/") && path[:len("/delete/")] == "/delete/" {
			// Handle DELETE via GET (from link click)
			idStr := path[len("/delete/"):]
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				http.Error(w, "Invalid album ID", http.StatusBadRequest)
				return
			}
			alb := Album{ID: id}
			_, err = deleteAlbum(alb)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			// Redirect to view page after deletion
			http.Redirect(w, r, "/view/", http.StatusSeeOther)
		}
		return
	}

	// Handle POST requests - process form submission
	if r.Method == "POST" {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Failed to parse form", http.StatusBadRequest)
			return
		}

		title := r.FormValue("title")
		artist := r.FormValue("artist")
		priceStr := r.FormValue("price")
		local_fg := r.FormValue("local_fg")

		var price float32
		_, err = fmt.Sscanf(priceStr, "%f", &price)
		if err != nil {
			http.Error(w, "Invalid price", http.StatusBadRequest)
			return
		}

		// Determine if this is create or edit based on URL
		if path == "/create/" || path == "/create" {
			// Create new album
			alb := Album{
				Title:   title,
				Artist:  artist,
				Price:   price,
				LocalFg: local_fg == "on", //Checkbox sends "on" if checked, else nothing
			}
			_, err = addAlbum(alb)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		} else if len(path) > len("/save/") {
			// Edit existing album
			idStr := path[len("/save/"):]
			id, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil {
				http.Error(w, "Invalid album ID", http.StatusBadRequest)
				return
			}

			alb := Album{
				ID:      id,
				Title:   title,
				Artist:  artist,
				Price:   price,
				LocalFg: local_fg == "on",
			}
			_, err = editAlbum(alb)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		http.Redirect(w, r, "/view/", http.StatusSeeOther) //prevents double submit
	}
}

// ------------database related - viewing-----------------//
// albumsByArtist queries for albums that have the specified artist name.
func albumsByArtist(name string) ([]Album, error) {
	// An albums slice to hold data from returned rows.
	var albums []Album

	var rows *sql.Rows
	var err error

	if (name == "") || (name == "null") {
		rows, err = db.Query("SELECT * FROM album")
	} else {
		rows, err = db.Query("SELECT * FROM album WHERE artist LIKE ?", "%"+name+"%")
	}

	if err != nil {
		return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
	}
	defer rows.Close()

	// Loop through rows, using Scan to assign column data to struct fields.
	for rows.Next() {
		var alb Album
		if err := rows.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price, &alb.LocalFg); err != nil {
			return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
		}
		albums = append(albums, alb)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("albumsByArtist %q: %v", name, err)
	}
	return albums, nil
}

// albumByID queries for the album with the specified ID.
func albumByID(id int64) (Album, error) {
	// An album to hold data from the returned row.
	var alb Album

	row := db.QueryRow("SELECT * FROM album WHERE id = ?", id)
	if err := row.Scan(&alb.ID, &alb.Title, &alb.Artist, &alb.Price, &alb.LocalFg); err != nil {
		if err == sql.ErrNoRows {
			return alb, fmt.Errorf("albumsById %d: no such album", id)
		}
		return alb, fmt.Errorf("albumsById %d: %v", id, err)
	}
	return alb, nil
}

// ------------database related - editing----------------//
// addAlbum adds the specified album to the database,
// returning the album ID of the new entry
func addAlbum(alb Album) (int64, error) {
	result, err := db.Exec("INSERT INTO album (title, artist, price, local_fg) VALUES (?, ?, ?, ?)", alb.Title, alb.Artist, alb.Price, alb.LocalFg)
	if err != nil {
		return 0, fmt.Errorf("addAlbum: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("addAlbum: %v", err)
	}
	return id, nil
}

func editAlbum(alb Album) (int64, error) {
	_, err := db.Exec("UPDATE album SET title=?, artist=?, price=?, local_fg=? WHERE id=?", alb.Title, alb.Artist, alb.Price, alb.LocalFg, alb.ID)
	if err != nil {
		return 0, fmt.Errorf("editAlbum: %v", err)
	}
	return alb.ID, nil
}

func deleteAlbum(alb Album) (int64, error) {
	_, err := db.Exec("DELETE FROM album WHERE id=?", alb.ID)
	if err != nil {
		return 0, fmt.Errorf("deleteAlbum: %v", err)
	}
	return alb.ID, nil
}

func main() {
	// Parse the template
	var err error
	tmpl, err = template.ParseFiles("page.html")
	if err != nil {
		log.Fatal(err)
	}

	// Capture connection properties.
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("DBUSER")
	cfg.Passwd = os.Getenv("DBPASS")
	cfg.Net = "tcp"
	cfg.Addr = "127.0.0.1:3306"
	cfg.DBName = "recordings"

	// Get a database handle.
	db, err = sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}

	pingErr := db.Ping()
	if pingErr != nil {
		log.Fatal(pingErr)
	}
	fmt.Println("Connected!")

	http.HandleFunc("/view/", viewHandler)
	http.HandleFunc("/view", viewHandler)
	http.HandleFunc("/create/", saveHandler)
	http.HandleFunc("/create", saveHandler)
	http.HandleFunc("/edit/", saveHandler)
	http.HandleFunc("/save/", saveHandler)
	http.HandleFunc("/delete/", saveHandler)
	http.Handle("/", http.FileServer(http.Dir(".")))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
