# Private bucket for exam PDFs, accessed by the container via S3 API.

resource "scaleway_object_bucket" "exams" {
  name = var.bucket_name
  tags = {
    app = "tenttiarkisto"
  }
}

resource "scaleway_object_bucket_acl" "exams" {
  bucket = scaleway_object_bucket.exams.id
  acl    = "private"
}

# IAM identity for the container to use the bucket.
resource "scaleway_iam_application" "s3" {
  name = "${var.container_name}-s3-app"
}

resource "scaleway_iam_api_key" "s3" {
  application_id = scaleway_iam_application.s3.id
  description    = "Object Storage credentials for the ${var.container_name} container"
}

resource "scaleway_iam_policy" "s3_access" {
  name           = "${var.container_name}-s3-access"
  description    = "Allow the app IAM identity to read/write exam objects in the project"
  application_id = scaleway_iam_application.s3.id
  rule {
    project_ids = [local.project_id]
    permission_set_names = [
      "ObjectStorageObjectsDelete",
      "ObjectStorageObjectsRead",
      "ObjectStorageObjectsWrite",
      "ObjectStorageBucketRead",
    ]
  }
}