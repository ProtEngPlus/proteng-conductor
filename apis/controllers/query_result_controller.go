package controllers

import (
	"strconv"
	"github.com/gin-gonic/gin"

	"github.com/protengplus/proteng-conductor/internal/conductor"
	"github.com/protengplus/proteng-conductor/repositories"
	"github.com/protengplus/proteng-conductor/utils/apiutil"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type QueryResultController struct {
	jobRepository		  repositories.JobRepository
	queryResultRepository repositories.QueryResultRepository
	conductor			  conductor.Conductor
}

func NewQueryResultController(jobRepository repositories.JobRepository, queryRepository repositories.QueryResultRepository, conductor conductor.Conductor) *QueryResultController {
	return &QueryResultController{jobRepository: jobRepository, queryResultRepository: queryRepository, conductor: conductor}
}

// GetAllQueryResults retrieves all query results
func (qr *QueryResultController) GetAllQueryResults(c *gin.Context) {
	query := map[string]interface{}{}
	if jobID, ok := query["job_id"].(string); ok {
		objectID, err := primitive.ObjectIDFromHex(jobID)
		if err == nil {
			query["job_id"] = objectID
		}
	}
	if isSelected := c.Query("is_selected"); isSelected != "" {
		if isSelectedBool, err := strconv.ParseBool(isSelected); err == nil {
			query["is_selected"] = isSelectedBool
		}
	}
	if organisms := c.Query("organisms"); organisms != ""{
		query["organisms"] = organisms
	}
	if percentIdentityFrom := c.Query("percentIdentityFrom"); percentIdentityFrom != "" {
		query["percent_identity_from"] = percentIdentityFrom
	}
	if percentIdentityTo := c.Query("percentIdentityTo"); percentIdentityTo != ""{
		query["percent_identity_to"] = percentIdentityTo
	}
	if eValuesFrom := c.Query("eValuesFrom"); eValuesFrom != "" {
        query["e_values_from"] = eValuesFrom
    }
	if eValuesTo := c.Query("eValuesTo"); eValuesTo != "" {
		query["e_values_to"] = eValuesTo
	}
	if queryCoverFrom := c.Query("queryCoverFrom"); queryCoverFrom!= "" {
        query["query_cover_from"] = queryCoverFrom
    }
	if queryCoverTo := c.Query("queryCoverTo"); queryCoverTo!= "" {
        query["query_cover_to"] = queryCoverTo
    }
	if sort := c.Query("sort"); sort != "" {
		query["sort"] = sort
	}
	if order := c.Query("order"); order != "" {
		switch order {
        case "asc":
            query["order"] = 1
        case "desc":
            query["order"] = -1
        }
	}

	query_results, err := qr.queryResultRepository.GetAll(query)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
        return
	}

	apiutil.ApiResponseOk(c, query_results)
}

// GetQueryResult retrieves a query result by ID
func (qr *QueryResultController) GetQueryResult(c *gin.Context) {
    id := c.Param("id")
    query_result, err := qr.queryResultRepository.FindById(id)
    if err != nil {
        apiutil.ApiResponseNotFound(c, err)
        return
    }

    apiutil.ApiResponseOk(c, query_result)
}

func (qr *QueryResultController) UpdateQueryResult(c *gin.Context) {
	id := c.Param("id")
	query_result, err := qr.queryResultRepository.FindById(id)
	if err != nil {
		apiutil.ApiResponseNotFound(c, err)
        return
	}
	err = c.BindJSON(&query_result)
	if err != nil {
		apiutil.ApiResponseErrorBadRequest(c, err, "error: invalid request body")
        return
	}

	err = qr.queryResultRepository.Update(id, query_result)
	if err != nil {
		apiutil.ApiResponseInternalServerError(c, err)
        return
	}

	apiutil.ApiResponseOk(c, query_result)
}