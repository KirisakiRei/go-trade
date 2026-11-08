package dto

type ILA2AnalysisRequest struct {
	Symbol string `form:"symbol" json:"symbol" binding:"required"`
	HTF    string `form:"htf,default=H4" json:"htf"`
	LTF    string `form:"ltf,default=M15" json:"ltf"`
	Limit  int    `form:"limit,default=100" json:"limit"`
}
