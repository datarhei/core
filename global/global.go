package global

var coreid string = ""

func SetCoreID(id string) {
	coreid = id
}

func GetCoreID() string {
	return coreid
}
