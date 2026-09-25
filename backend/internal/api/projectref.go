package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"visora-backend/internal/db"
)

// ResolveProjectID rewrites /projects/:id from the public random id to the
// internal numeric key so existing handlers keep working.
func ResolveProjectID() gin.HandlerFunc {
	return func(c *gin.Context) {
		ref := c.Param("id")
		if ref == "" {
			c.Next()
			return
		}
		project, err := db.FindProjectByRef(ref)
		if err != nil {
			c.Next()
			return
		}
		for i := range c.Params {
			if c.Params[i].Key == "id" {
				c.Params[i].Value = fmt.Sprintf("%d", project.ID)
			}
		}
		c.Set("project", project)
		c.Next()
	}
}
