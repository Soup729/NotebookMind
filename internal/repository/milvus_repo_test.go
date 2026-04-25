package repository

import (
	"strings"
	"testing"

	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

func TestValidateMilvusCollectionSchema(t *testing.T) {
	collection := &entity.Collection{
		Schema: entity.NewSchema().
			WithName("pdf_chunks").
			WithAutoID(true).
			WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
			WithField(entity.NewField().WithName("user_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
			WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
			WithField(entity.NewField().WithName("file_name").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)).
			WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
			WithField(entity.NewField().WithName("page_num").WithDataType(entity.FieldTypeInt64)).
			WithField(entity.NewField().WithName("chunk_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
			WithField(entity.NewField().WithName("chunk_role").WithDataType(entity.FieldTypeVarChar).WithMaxLength(16)).
			WithField(entity.NewField().WithName("parent_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
			WithField(entity.NewField().WithName("section_path").WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024)).
			WithField(entity.NewField().WithName("bbox").WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
			WithField(entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
			WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(1536)),
	}

	if err := validateMilvusCollectionSchema(collection, 1536); err != nil {
		t.Fatalf("expected compatible schema, got error: %v", err)
	}
}

func TestValidateMilvusCollectionSchemaMissingField(t *testing.T) {
	collection := &entity.Collection{
		Schema: entity.NewSchema().
			WithName("pdf_chunks").
			WithAutoID(true).
			WithField(entity.NewField().WithName("id").WithDataType(entity.FieldTypeInt64).WithIsPrimaryKey(true).WithIsAutoID(true)).
			WithField(entity.NewField().WithName("document_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
			WithField(entity.NewField().WithName("file_name").WithDataType(entity.FieldTypeVarChar).WithMaxLength(512)).
			WithField(entity.NewField().WithName("chunk_index").WithDataType(entity.FieldTypeInt64)).
			WithField(entity.NewField().WithName("page_num").WithDataType(entity.FieldTypeInt64)).
			WithField(entity.NewField().WithName("chunk_type").WithDataType(entity.FieldTypeVarChar).WithMaxLength(32)).
			WithField(entity.NewField().WithName("chunk_role").WithDataType(entity.FieldTypeVarChar).WithMaxLength(16)).
			WithField(entity.NewField().WithName("parent_id").WithDataType(entity.FieldTypeVarChar).WithMaxLength(128)).
			WithField(entity.NewField().WithName("section_path").WithDataType(entity.FieldTypeVarChar).WithMaxLength(1024)).
			WithField(entity.NewField().WithName("bbox").WithDataType(entity.FieldTypeVarChar).WithMaxLength(256)).
			WithField(entity.NewField().WithName("text").WithDataType(entity.FieldTypeVarChar).WithMaxLength(65535)).
			WithField(entity.NewField().WithName("vector").WithDataType(entity.FieldTypeFloatVector).WithDim(1536)),
	}

	err := validateMilvusCollectionSchema(collection, 1536)
	if err == nil {
		t.Fatal("expected schema validation to fail")
	}
	if !strings.Contains(err.Error(), `missing field "user_id"`) {
		t.Fatalf("expected missing user_id error, got: %v", err)
	}
}

func TestBuildMilvusExpr(t *testing.T) {
	expr := buildMilvusExpr(VectorSearchOptions{
		UserID:      "user-1",
		DocumentIDs: []string{"doc-1", "doc-2"},
	})

	expected := `user_id == "user-1" && document_id in ["doc-1","doc-2"]`
	if expr != expected {
		t.Fatalf("unexpected expression: got %s want %s", expr, expected)
	}
}
