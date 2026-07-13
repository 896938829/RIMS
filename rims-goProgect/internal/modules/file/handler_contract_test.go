package file

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

func TestFileHandlerReorderContractReturnsStablePositionsAndAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	businessID := uint(9)
	repo := &mutationFileRepo{binding: []FileAttachment{mutationFile(1, businessID, 7, 0), mutationFile(2, businessID, 7, 1)}}
	logger := &fileHandlerAuditLogger{}
	handler := NewHandler(NewFileService(repo, &mutationStorage{}, 10, ".txt", "/files/%d/download", nil), logger)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPut, "/files/reorder", bytes.NewBufferString(`{"businessType":"doc_attachment","businessId":9,"fileIds":[2,1]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.CtxKeyUserID, uint(7))

	handler.Reorder(c)

	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte(`"position":0`)) || !bytes.Contains(recorder.Body.Bytes(), []byte(`"position":1`)) {
		t.Fatalf("status/body = %d/%s", recorder.Code, recorder.Body.String())
	}
	if len(logger.entries) != 1 || logger.entries[0].Action != audit.ActionUpdate || logger.entries[0].After["businessID"] != uint(9) {
		t.Fatalf("audit entries = %#v", logger.entries)
	}
}

func TestFileHandlerReplaceContractPreservesIDAndUsesIdempotencyRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	businessID := uint(9)
	original := mutationFile(4, businessID, 7, 3)
	original.ObjectKey = "old.txt"
	repo := &mutationFileRepo{aclFileRepoStub: aclFileRepoStub{files: map[uint]*FileAttachment{4: &original}}}
	logger := &fileHandlerAuditLogger{}
	handler := NewHandler(NewFileService(repo, &mutationStorage{}, 10, ".txt", "/files/%d/download", nil), logger)

	body, contentType := fileUploadMultipartBody(t, nil, "file", "new.txt", "new body")
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/files/4/replace", body)
	c.Request.Header.Set("Content-Type", contentType)
	c.Params = gin.Params{{Key: "id", Value: "4"}}
	c.Set(types.CtxKeyUserID, uint(7))
	handler.Replace(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status/body = %d/%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data FileResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.ID != 4 || response.Data.Position != 3 {
		t.Fatalf("response/error = %#v/%v", response, err)
	}
	if len(logger.entries) != 1 || logger.entries[0].Action != audit.ActionUpdate {
		t.Fatalf("audit entries = %#v", logger.entries)
	}

	idemCalls := 0
	router := gin.New()
	api := router.Group("/api/v1")
	RegisterRoutes(api, handler, func(c *gin.Context) { c.Set(types.CtxKeyUserID, uint(7)); c.Next() }, func(c *gin.Context) { idemCalls++; c.Next() }, func(string) gin.HandlerFunc { return func(c *gin.Context) { c.Next() } })
	request := httptest.NewRequest(http.MethodPost, "/api/v1/files/4/replace", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)
	if idemCalls != 1 {
		t.Fatalf("idempotency calls = %d, want 1", idemCalls)
	}
}
