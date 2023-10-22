package fgprof

//import (
//	"github.com/felixge/fgprof"
//	"net/http"
//	_ "net/http/pprof"
//)
//
//type Options struct {
//	Enable bool
//	// default "/debug/fgprof"
//	Addr string
//	// default 6060
//	Port int
//}
//
//func (*Options) Init() error {
//	http.DefaultServeMux.Handle("/debug/fgprof", fgprof.Handler())
//	go func() {
//		err := http.ListenAndServe(":6060", nil)
//		if err != nil {
//			return
//		}
//	}()
//	return nil
//}
