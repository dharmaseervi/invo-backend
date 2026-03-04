package middleware

import (
	"database/sql"

	"github.com/gin-gonic/gin"
)

func DBTransactionMiddleware(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		tx, err := db.Begin()
		if err != nil {
			c.AbortWithStatusJSON(500, gin.H{
				"error": "failed to start transaction",
			})
			return
		}

		c.Set("tx", tx)

		c.Next()

		if c.IsAborted() {
			_ = tx.Rollback()
			return
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
		}
	}
}
