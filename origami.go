/*********************************************************
*   program:    origami
*   desc:       gets toner levels and other printer info for printers at etown college
*   files:      origami.go
*   author:     rory dudley (pinecat)
*********************************************************/

/* package */
package main

/* imports */
import (
    "fmt" // for printing out info
    "log" // for logging info
    "os" // for opening files and getting cmdline args
    "bufio" // for reading in files
    "time" // for sleeping on an interval and getting the date-time
    "sync" // for syncing go routines
    "strings" // for parsing (splitting) strings when reading from conf file
    "sort" // for sorting maps
    "strconv" // for converting strings to ints and ints to strings
    "regexp" // for parsing out percents and other info using regex
    "net/http" // for routing and get requests
    "crypto/tls" // for ignoring bad ssl certs
    _"io/ioutil" // for reading text
    "html/template" // html template for the index page
    "github.com/PuerkitoBio/goquery" // for parsing html
)

/* globals */
var (
    percentRegex *regexp.Regexp // for holding regular expression to parse toner percent
    cartridgeRegex *regexp.Regexp // for holding regular expression to parse cartridge type
    wg sync.WaitGroup // for syncing go routines
    pd PageData
    indexPage string = `
        <!DOCTYPE html>
        <head>
            <title>OrigamiV2</title>
            <link rel="icon" type="image/png" href="https://openclipart.org/image/2400px/svg_to_png/202202/star.png"/>
            <style>
                body {background-color: #3560A5;}
                #heading {
                    background-color: #292A47;
                    color: white;
                    width: 100%;
                    padding: 20px;
                    padding-bottom: -20px;
                    margin-top: -10px;
                    margin-left: -10px;
                    vertical-align: top;
                }
                #heading h4 {
                    float: right;
                    margin: -50px;
                    margin-right: 20px;
                }
                #heading a {
                    color: white;
                    text-decoration: none;
                }
                #heading a:hover {
                    color: #C65959;
                }
                #last h3 {
                    color: #232326;
                }
                #last h4 {
                    color: #232326;
                }
                #data tbody td h3 {
                    color: white;
                    font-weight: normal;
                }
                #data thead th h3 {
                    color: white;
                    font-weight: bold;
                }
                #data table {
                    border: 2px solid;
                }
                #data table a {
                    text-decoration: none;
                    color: white;
                }
                #data table a:hover {
                    color: #2F2F35;
                }
                #data thead th {
                    border: 2px solid;
                }
                #data tbody td {
                    border: 2px solid;
                    padding-top: -10px;
                    padding-bottom: -10px;
                    padding-left: 80px;
                    padding-right: 80px;
                }
            </style>
        </head>

        <body>
            <div id="heading">
                <h1>Origami V2</h1>
                <h4>By Rory Dudley (aka pinecat)</h4><br>
                <h4><a href="https://www.gitlab.com/pinecat/origamiv2" target="_blank">Gitlab</a></h4>
            </div>

            <div id="last">
                <h3>Last Updated @{{ .Last }}</h3>
                <h4>Next Update @{{ .Next }}</h4>
            </div>

            <div id="data">
                <table>
                    <thead>
                        <th align="center"><h3>Printer Name</h3></th>
                        <th align="center"><h3>Toner Level</h3></th>
                        <th align="center"><h3>Cartridge Type</h3></th>
                    </thead>

                    {{ range $index, $data := .Printers }}
                    <tbody>
                        <td align="center"><h3><a href="{{ $data.Addr }}" target="_blank">{{ $data.Name }}</a></h3></td>
                        <td align="center"><h3>{{ $data.Toner }}</h3></td>
                        <td align="center"><h3>{{ $data.Cart }}</h3></td>
                    </tbody>
                    {{ end }}
                </table>
            </div>
        </body>
    `
)

/* structs */
type PageData struct {
    Printers    []PrinterData
    Last        string
    Next        string
}

type PrinterData struct {
    Name    string
    Addr    string
    Toner   string
    Cart    string
}

/*
    dispError: checks if there is an error, displays message and error if err is not nil, does not exit program
    params:     msg - message to display if err is not nil
                err - the error in question (possibly nil)
    returns:    void
*/
func dispError(msg string, err error) {
    if err != nil {
        log.Println("MSG: ", msg, " ERR: ", err)
    }
}

/*
    checkError: checks if there is an error, displays message and error if err is not nil, then exits the program
    params:     msg - message to display if err is not nil
                err - the error in question (possibly nil)
    returns:    void
*/
func checkError(msg string, err error) {
    if err != nil {
        log.Fatal("MSG: ", msg, " ERR: ", err)
    }
}

/*
    readInPrinters: read in the configuration file for origami (includes printers and their ips, html tags/classes to search for, and the interval to grab data at)
    params:         filepath - filepath of the configuration file ("origami.conf" by default)
    returns:        printers - a map of the printers and their ips
                    search - an array of html tags/classes to search through
                    interval - the interval at which to collect data (in minutes)
*/
func readInPrinters(filepath string) (map[string]string, []string, int, string) {
    file, err := os.Open(filepath) // open the file specified
    checkError("Could not read in printer file!", err) // check for error when opening the file
    defer file.Close() // close the file at the end of the method

    scanner := bufio.NewScanner(file) // create new scanner to read the file

    printers := make(map[string]string) // create a map for printers and their ips
    var search []string // create array for classes to search
    var interval int // create int for the interval
    var port string // create string for the port

    for scanner.Scan() { // keep scanning
        if scanner.Text() == "[SEARCH]" { // if we get to the next section in the conf file...
            break // ...break from this loop
        }
        if scanner.Text() != "[PRINTERS]" && scanner.Text() != "\n" && scanner.Text() != "" { // don't pickup uneeded text
            s := strings.Split(scanner.Text(), "=") // otherwise, split the string
            printers[s[0]] = s[1] // then add them to the map
        }
    }

    for scanner.Scan() { // keep scanning
        if scanner.Text() == "[INTERVAL]" { // if we get to the next section in the conf file...
            break // ...break from this loop
        }
        if scanner.Text() != "[SEARCH]" && scanner.Text() != "\n" && scanner.Text() != "" { // don't pickup uneeded text
            search = append(search, scanner.Text()) // add to the search array
        }
    }

    for scanner.Scan() { // keep scanning
        if scanner.Text() == "[PORT]" { // if we get to the next section in the conf file...
            break // ...break from this loop
        }
        if scanner.Text() != "[INTERVAL]" && scanner.Text() != "\n" && scanner.Text() != "" { // don't pickup uneeded text
            interval, err = strconv.Atoi(strings.Split(scanner.Text(), "=")[1]) // split the string and set the interval
            checkError("Invalid interval in configuration file!", err) // check for errors with the specified interval
            if interval < 1 { // interval cannot be less than 1, so...
                log.Fatal("MSG: Interval may not be less than 1 minute!\n") // log a fatal error if it is less than 1, and exit the program
            }
        }
    }

    for scanner.Scan() { // keep scanning
        if scanner.Text() != "[PORT]" && scanner.Text() != "\n" && scanner.Text() != "" { // don't pickup uneeded text
            port = ":" + strings.Split(scanner.Text(), "=")[1] // get the port
        }
    }

    return printers, search, interval, port // return values
}

/*
    sortMap:    creates slice of string keys to sort a map
    params:     m - the map to be sorted
    returns:    keys - the slice of string keys of the map
*/
func sortMap(m map[string]string) []string {
    keys := make([]string, 0)
    for name, _ := range m {
        keys = append(keys, name)
    }
    sort.Strings(keys)
    return keys
}

/*
    getPrinterData: gets printer data from web page and parses it
    params:         ip - ip address of printer
                    search - the array of html tags/classes to query through
    returns:        toner - percent of remaining toner
                    cartridge - the printer cartridge type
*/
func getPrinterData(ip string, search []string, toner *string, cartridge *string) {
    resp, err := http.Get("http://" + ip) // get the html of the printer status page
    dispError("Could not access printer status page!", err) // display error if we could not access the page
    defer resp.Body.Close() // close response body at the end or exit of this function

    doc, err := goquery.NewDocumentFromResponse(resp) // generate document from the http response to parse through
    dispError("Could not create query-able document!", err) // display error if we could not create the document

    //var toner string // string to hold toner percent
    //var cartridge string // string to hold cartridge type

    var block string
    for _, name := range search {
        doc.Find(name).EachWithBreak(func(i int, s *goquery.Selection) bool{
            block, _ = s.Html()
            *toner = percentRegex.FindString(block) // parse the text to find the toner percent, and update toner
            *cartridge = cartridgeRegex.FindString(block) // parse the text to find the cartridge type, and update cartridge
            return false
        })
    }


}

/*
    indexHandler:   handles the index page of the web app
    params:         w - http respsonse writer
                    r - http request
    returns:        void
*/
func indexHandler(w http.ResponseWriter, r *http.Request) {
    t, _ := template.New("webpage").Parse(indexPage) // parse embeded index page
    t.Execute(w, pd) // serve the index page (html template)
}

/*
    help:       prints a help menu
    params:     n/a
    returns:    void
*/
func help() {
    fmt.Printf("ORIGAMI\n")
    fmt.Printf("\tA web app that checks the toner levels of printers at the Elizabethtown College campus.\n\n")
    fmt.Printf("USAGE\n")
    fmt.Printf("\tUsage: origami [-f filepath | -h]\n\n")
    fmt.Printf("OPTIONS\n")
    fmt.Printf("\t-f: specify the filepath of the config file (\"./origami.conf\" by default)\n")
    fmt.Printf("\t-h: this menu\n\n")
    fmt.Printf("AUTHOR\n")
    fmt.Printf("\tRory Dudley (aka pinecat: https://github.com/pinecat/origamiv2)\n\n")
    fmt.Printf("EOF\n")
}

/*
    main:       main function of the program
    params:     n/a
    returns:    void
*/
func main() {
    filepath := "origami.conf" // setup default filepath for reading configuration file
    if len(os.Args) == 3 && os.Args[1] == "-f" { // read in different filepath if specified by user at cmdline
        filepath = os.Args[2] // update the filepath
    } else if len(os.Args) > 1 && os.Args[1] == "-f" { // if format for -f flag is not correct...
        fmt.Printf("Usage: %s [-f filepath | -h]\n", os.Args[0]) // print a usage message
        return // and exit the program
    } else if len(os.Args) > 1 && os.Args[1] == "-h" { // if flag is -h...
        help() // ...print a help menu
        return // and exit the program
    }

    printers, search, interval, port := readInPrinters(filepath) // read in information from configuration file
    keys := sortMap(printers) // generate sorted string key slice of the printers map
    pd.Printers = make([]PrinterData, len(keys)) // initialize PrinterData array for our page data
    http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // tell http get requests to ignore bad ssl certs
    percentRegex, _ = regexp.Compile(`\d\d%|\d%`) // regular expression for getting toner percents
    cartridgeRegex, _ = regexp.Compile(`[A-Z0-9]{6}`) // regular expression for getting cartridge type
    _ = interval

    http.HandleFunc("/", indexHandler) // handle the index page
    go http.ListenAndServe(port, nil) // start the web server
    log.Printf("Server started on port %s!\n", strings.Split(port, ":")[1])

    for i, name := range keys {
        pd.Printers[i].Name = name
    }

    for {
        for i, name := range keys { // for all the printers in our map
            var toner string // variable to hold toner percent
            var cartridge string // variable to hold cartridge type
            getPrinterData(printers[name], search, &toner, &cartridge) // run a go routine

            pd.Printers[i].Addr = "http://" + printers[name] // set address of printer via ip address
            pd.Printers[i].Toner = toner // set toner
            pd.Printers[i].Cart = cartridge // set cartridge type
        }
        pd.Last = time.Now().Format("2006-01-02_15:04:05") // update when the last check was
        pd.Next = time.Now().Add(time.Minute * time.Duration(interval)).Format("2006-01-02_15:04:05") // calculate when the next check will be
        time.Sleep(time.Minute * time.Duration(interval)) // sleep for the interval specified
    }
}
