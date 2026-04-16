package payloads

// TransactionBeginPayload is the payload for the vdb.transaction.begin pipeline.
type TransactionBeginPayload struct {
	ConnectionID uint32
	ReadOnly     bool
}

// TransactionCommitPayload is the payload for the vdb.transaction.commit pipeline.
type TransactionCommitPayload struct {
	ConnectionID uint32
}

// TransactionRollbackPayload is the payload for the vdb.transaction.rollback pipeline.
type TransactionRollbackPayload struct {
	ConnectionID uint32
	Savepoint    string
}
