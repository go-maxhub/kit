package gin

import (
	"github.com/gin-gonic/gin"
)

type Config struct {
	Default bool
	Blank   bool
}

func (c *Config) NewDefaultGin() *gin.Engine {
	cl := gin.Default()
	return cl
}

func (c *Config) NewBlankGin() *gin.Engine {
	cl := gin.New()
	return cl
}
