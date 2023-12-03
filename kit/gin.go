package kit

import "github.com/gin-gonic/gin"

type GinConfig struct {
	Default bool
	Blank   bool
}

func (c *GinConfig) NewDefaultGin() *gin.Engine {
	cl := gin.Default()
	return cl
}

func (c *GinConfig) NewBlankGin() *gin.Engine {
	cl := gin.New()
	return cl
}
