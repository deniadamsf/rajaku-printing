package handler

type rejectRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

type revokeRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}

type reinstateRequest struct {
	Reason string `json:"reason" binding:"required,min=3,max=500"`
}
