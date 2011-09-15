package tpbitcalc

import (
    "fmt"
    "http"
    //"math"
    "appengine"
    "appengine/urlfetch"
    "appengine/taskqueue"
    "io/ioutil"
    "strconv"
    "template"
    "os"
    "log"
    "time"
    "json"
)
//TODO: change 50.0 to a variable?

var CurrentDifficulty float32 //current difficulty of the Bitcoin network
var CurrentExchangeRate float32 //current USD exchange rate (24h average updated about once an hour from bitcoin charts)

var DefaultCalc Calculations //default data to display to the user
var CalcTemplate *template.Template //HTML template to use

var LastCheckTime int64 //last time the current data was checked

const TIMEINTERVALBETWEENCHECKS = int64(60*60)//one hour interval between checks


//function to calculate how much the person is earning each day
func calculateDailyReward(difficulty float32, hashrate float32, reward float32) float32{
	result:=reward//f.e. 50 bitcoins
	result*=hashrate//multiplying by miner speed
	result/=difficulty//dividing by difficulty
	result/=1<<32//dividing by 2^32
	result*=60*60*24//seconds in a day

	return result
}

//the structure to store the data for calculation and displaying
type Calculations struct {
	//basic variables
	Difficulty float32
	HashRate float32
	ExchangeRate float32
	BitcoinsPerBlock float32
	
	RigCost float32
	PowerConsumption float32
	PowerCost float32
	
	//their string representation to be displayed in HTML (HTML has problems parsing big and small numbers, displying them in scientific notation [f.e. 1e9])
	DifficultyStr string
	HashRateStr string
	ExchangeRateStr string
	BitcoinsPerBlockStr string
	
	RigCostStr string
	PowerConsumptionStr string
	PowerCostStr string
	
	//variable to indicate whether or not to display results
	DisplayResults bool
	
	//reward strings to be displayed in HTML. Don`t need to keep dat in float32, since it won't be used anymore
	//Rewards in Bitcoins
	HourlyRewardBC string
	DailyRewardBC string
	WeeklyRewardBC string
	MonthlyRewardBC string
	AnnualRewardBC string
	
	//Rewards in Dollars
	HourlyRewardD string
	DailyRewardD string
	WeeklyRewardD string
	MonthlyRewardD string
	AnnualRewardD string
	
	//how long does it take to generate a block
	TimeToGenerateABlock string
	BlocksGeneratedPerYear string
	
	//results that take power cost into consideration
	MiningCost string
	HourlyNetProfit string
	DailyNetProfit string
	WeeklyNetProfit string
	MonthlyNetProfit string
	AnnualNetProfit string
	BreakEvenTime string
}


//first function to get called
func init() {
    http.HandleFunc("/", hello)//main page to display
    http.HandleFunc("/recalculate", recalculate)//page with results
    http.HandleFunc("/keepDataUpToDate", keepDataUpToDate)//function for keeping data up to date
    
    //initialize variables with dummy values
    LastCheckTime=0
    
    CurrentDifficulty=1.0
	CurrentExchangeRate=1.0
    
    //initialize the default values
    DefaultCalc.Difficulty=CurrentDifficulty
    DefaultCalc.HashRate=100.0
    DefaultCalc.ExchangeRate=CurrentExchangeRate
	DefaultCalc.BitcoinsPerBlock=50.0
    DefaultCalc.DisplayResults=false
    
    DefaultCalc.RigCost=0.0
	DefaultCalc.PowerConsumption=200.0
	DefaultCalc.PowerCost=0.1
	
	
	DefaultCalc.DifficultyStr=fmt.Sprintf("%.2f", DefaultCalc.Difficulty)
	DefaultCalc.HashRateStr=fmt.Sprintf("%.2f", DefaultCalc.HashRate)
	DefaultCalc.ExchangeRateStr=fmt.Sprintf("%.2f", DefaultCalc.ExchangeRate)
	DefaultCalc.BitcoinsPerBlockStr=fmt.Sprintf("%.2f", DefaultCalc.BitcoinsPerBlock)
	
	DefaultCalc.RigCostStr=fmt.Sprintf("%.2f", DefaultCalc.RigCost)
	DefaultCalc.PowerConsumptionStr=fmt.Sprintf("%.2f", DefaultCalc.PowerConsumption)
	DefaultCalc.PowerCostStr=fmt.Sprintf("%.2f", DefaultCalc.PowerCost)
    
    
    err:=os.Error(nil)
    CalcTemplate = template.New(nil)
    CalcTemplate.SetDelims("{{", "}}")//changing Delims from {} to {{}} to be able to run the Flattr script
	err = CalcTemplate.ParseFile("tpbitcalc/calc.html")//parses the HTML file. Produces errors if default delims are used because of Flattr code
    
    //logs an error
    if err!=nil {
    	log.Print("init err ", err)
    	//http.Error(w, err.String(), http.StatusInternalServerError)
		return
    
    }
}

//function to calculate users's request
func calculateEverything(Calc Calculations) Calculations{
	result := calculateDailyReward(Calc.Difficulty, Calc.HashRate*1000000, Calc.BitcoinsPerBlock)
	
	//Rewards in Bitcoins
	Calc.HourlyRewardBC=fmt.Sprintf("%f", result/24.0)
	Calc.DailyRewardBC=fmt.Sprintf("%f", result)
	Calc.WeeklyRewardBC=fmt.Sprintf("%f", result*7.0)
	Calc.MonthlyRewardBC=fmt.Sprintf("%f", result*30.0)
	Calc.AnnualRewardBC=fmt.Sprintf("%f", result*365.0)
	
	//Rewards in Dollars
	Calc.HourlyRewardD = fmt.Sprintf("%f", result/24.0*Calc.ExchangeRate)
	Calc.DailyRewardD = fmt.Sprintf("%f", result*Calc.ExchangeRate)
	Calc.WeeklyRewardD = fmt.Sprintf("%f", result*7*Calc.ExchangeRate)
	Calc.MonthlyRewardD = fmt.Sprintf("%f", result*30*Calc.ExchangeRate)
	Calc.AnnualRewardD = fmt.Sprintf("%f", result*365.0*Calc.ExchangeRate)
	
	Calc.TimeToGenerateABlock=fmt.Sprintf("%.1f", Calc.BitcoinsPerBlock/result)
	Calc.BlocksGeneratedPerYear=fmt.Sprintf("%.3f", result*365.0/Calc.BitcoinsPerBlock)
	
	result=result/24.0*Calc.ExchangeRate-Calc.PowerConsumption/1000.0*Calc.PowerCost
	
	Calc.MiningCost = fmt.Sprintf("%f", Calc.PowerConsumption/1000.0*Calc.PowerCost)
	Calc.HourlyNetProfit = fmt.Sprintf("%f", result)
	Calc.DailyNetProfit = fmt.Sprintf("%f", result*24.0)
	Calc.WeeklyNetProfit = fmt.Sprintf("%.2f", result*24.0*7.0)
	Calc.MonthlyNetProfit = fmt.Sprintf("%.2f", result*24.0*30.0)
	Calc.AnnualNetProfit = fmt.Sprintf("%.2f", result*24.0*365.0)
	
	if(result<0.0){//not to display Break even after -1 days
		Calc.BreakEvenTime="Never"
	}else{
		Calc.BreakEvenTime=fmt.Sprintf("%.1f", Calc.RigCost/(result*24.0))
	}
	
	Calc.DifficultyStr=fmt.Sprintf("%.2f", Calc.Difficulty)
	Calc.HashRateStr=fmt.Sprintf("%.2f", Calc.HashRate)
	Calc.ExchangeRateStr=fmt.Sprintf("%.2f", Calc.ExchangeRate)
	Calc.BitcoinsPerBlockStr=fmt.Sprintf("%.2f", Calc.BitcoinsPerBlock)
	
	Calc.RigCostStr=fmt.Sprintf("%.2f", Calc.RigCost)
	Calc.PowerConsumptionStr=fmt.Sprintf("%.2f", Calc.PowerConsumption)
	Calc.PowerCostStr=fmt.Sprintf("%.2f", Calc.PowerCost)
	
	return Calc
}

//page that calculates everything for the user
func recalculate(w http.ResponseWriter, r *http.Request) {
	//log.Print("recalculate")
	
	var Calc Calculations//new calculations instance
	
	//getting data from request
	Calc.Difficulty, _=strconv.Atof32(r.FormValue("difficulty"))
    Calc.HashRate, _=strconv.Atof32(r.FormValue("hashrate"))
    Calc.ExchangeRate, _=strconv.Atof32(r.FormValue("exchangerate"))
    Calc.DisplayResults=true//set to true to display the second table
    
	Calc.RigCost, _=strconv.Atof32(r.FormValue("rigcost"))
    Calc.PowerConsumption, _=strconv.Atof32(r.FormValue("powerconsumption"))
    Calc.PowerCost, _=strconv.Atof32(r.FormValue("powercost"))
    Calc.BitcoinsPerBlock, _=strconv.Atof32(r.FormValue("bitcoinsperblock"))
    
    Calc = calculateEverything(Calc)//perform aditional calculations

    CalcTemplate.Execute(w, Calc)//display the page
}


//main page to call
func hello(w http.ResponseWriter, r *http.Request) {
	//log.Print("hello")

	go startDataChecking(r)//runs the function that makes sure the current data is updating

    CalcTemplate.Execute(w, DefaultCalc)//displays the default values
}



//function to fetch difficulty from the blockexplorer
func fetchDifficulty(c appengine.Context) float32{

	client := urlfetch.Client(c)
    resp, err := client.Get("http://blockexplorer.com/q/getdifficulty")
    if err != nil {
		log.Print("fetchDifficulty err %s", err.String())
        //http.Error(w, err.String(), http.StatusInternalServerError)
        return -1.0
    }
    
    //read response
    bs, _ := ioutil.ReadAll(resp.Body)

	//convert string to float
	difficulty, _:=strconv.Atof32(string(bs))
	
	return difficulty
}

//function that fetches the exchange rate from bitcoincharts
func getExchangeRate(c appengine.Context) float32{
	
	client := urlfetch.Client(c)
    resp, err := client.Get("http://bitcoincharts.com/t/weighted_prices.json")
    if err != nil {
		log.Print("getExchangeRate err ", err)
        return -1.0
    }
    
    //decode the json
    dec := json.NewDecoder(resp.Body)
    
    var f interface{}
    if err2 := dec.Decode(&f); err != nil {
        log.Println("getExchangeRate err2 ", err2)
        return -1.0
    }

	m := f.(map[string]interface{})
	//get the value for USD 24h
	tmptocheck:=m["USD"].(map[string]interface{})["24h"]
	
	switch v := tmptocheck.(type) {
	case string://if value is string, it converts it
		newValue, _:=strconv.Atof32(tmptocheck.(string))
		//log.Print(newValue)
		return newValue
	default://otherwise returns a false value (in case no data is fetched or something)	
	    return -1.0
	}
	
    return -1.0
}

//function that checks data such as Difficulty and exchange rate
func checkData(c appengine.Context){
	
	//check the difficulty
	diff:=fetchDifficulty(c)
	if diff>0.0 {
		CurrentDifficulty=diff
		DefaultCalc.Difficulty=CurrentDifficulty
		DefaultCalc.DifficultyStr=fmt.Sprintf("%.2f", DefaultCalc.Difficulty)
	}
	//check the exchange rate
	rate:=getExchangeRate(c)
	if rate>0.0{
		CurrentExchangeRate=rate
		DefaultCalc.ExchangeRate=CurrentExchangeRate
		DefaultCalc.ExchangeRateStr=fmt.Sprintf("%.2f", DefaultCalc.ExchangeRate)
	}
}

//function that starts the data checking, making sure only one instance of the data checker is running
func startDataChecking(r *http.Request){
	//log.Print("startDataChecking")
	
	if(LastCheckTime+TIMEINTERVALBETWEENCHECKS<time.UTC().Seconds()){
    	c := appengine.NewContext(r)
		taskqueue.Purge(c, "")//clears any other running data checking tasks, if there are any
		
	    t := taskqueue.NewPOSTTask("/keepDataUpToDate", nil)//creates a new task
	    //t.Delay=1e7
	    if _, err4 := taskqueue.Add(c, t, ""); err4 != nil {//runs the task
	        log.Print ("startDataChecking err4: %v", err4)
	        return
	    }
	    //log.Print("startDatChecking end")
	}
}

//function that checks data periodically and schedules itself again
func keepDataUpToDate(w http.ResponseWriter, r *http.Request){
	//log.Print("keepDataUpToDate")

    //check if the function isn't called too often
    //for example by calling this function directly by something other than the program itself
    if(LastCheckTime+TIMEINTERVALBETWEENCHECKS/2<time.UTC().Seconds()){
    	c := appengine.NewContext(r)
		LastCheckTime=time.UTC().Seconds()//logs the last check time
		
		checkData(c)//checks the data
		
		taskqueue.Purge(c, "")//clears any other running data checking tasks, if there are any
		
	    t := taskqueue.NewPOSTTask("/keepDataUpToDate", nil)//creates a new task
	    t.Delay=TIMEINTERVALBETWEENCHECKS//schedules it once an hour
	    if _, err4 := taskqueue.Add(c, t, ""); err4 != nil {
	        log.Print ("startDataChecking err4: %v", err4)
	        return
	    }
	    //log.Print("keepDataUpToDate end")
	}
}