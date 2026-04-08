package daos

type Instance struct {
	ID            int    `dynamodbav:"id"`
	ContainerName string `dynamodbav:"container_name"`
	Description   string `dynamodbav:"description"`
	DirName       string `dynamodbav:"dir_name"`
	KeepInventory bool   `dynamodbav:"keep_inventory"`
	Prefix        string `dynamodbav:"prefix"`
	S3Bucket      string `dynamodbav:"s3_bucket"`
	Active        bool   `dynamodbav:"active"`
	WorkingPath   string `dynamodbav:"working_path"`
}

type Save struct {
	ID         int    `dynamodbav:"id"`
	Filename   string `dynamodbav:"filename"`
	Deleted    bool   `dynamodbav:"deleted"`
	Size       int64  `dynamodbav:"size"`
	CreatedAt  int64  `dynamodbav:"created_at"`
	InstanceID int    `dynamodbav:"instance_id"`
}
