package ginprofiler

//import (
//	"github.com/gin-contrib/pprof"
//	"github.com/gin-gonic/gin"
//	"github.com/pkg/errors"
//)
//
//type Options struct {
//	Enable bool
//	Addr   string
//	Port   int
//}
//
//func (*Options) Init() error {
//	router := gin.Default()
//	pprof.Register(router)
//	err := router.Run(":8080")
//	if err != nil {
//		return errors.Wrap(err, "init gin pprof")
//	}
//	return nil
//}
