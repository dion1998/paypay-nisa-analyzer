export type Fund = { id:string; name:string; manager:string; nisaTsumitate:boolean; nisaGrowth:boolean; paypayUrl:string; historyUrl?:string; trustFeeRate?:number; trustFeeSource?:string; refreshedAt:string };
export type PricePoint = { fundId:string; date:string; nav:number; sourceUrl:string };
export type Insight = { id:number; fundId:string; publisher:string; title:string; summary:string; sourceUrl:string; publishedAt:string };
export type HoldingCriteria = { years:number; sampleCount:number; observedSampleCount:number; usesBootstrap:boolean; evidenceLevel:string; cpiAsOf:string; cpiAvailable:boolean; realSuccessRate:number; successRateLowerBound:number; p10RealReturn:number; expectedShortfall10:number; maximumDrawdown:number; worstPathDrawdown:number; successRateThreshold:number; successLowerBoundThreshold:number; p10Threshold:number; expectedShortfallThreshold:number; maximumDrawdownThreshold:number; requiredStableHorizons:number; stableHorizonsPassed:number; riskCriteriaPassed:boolean; passed:boolean; failedReasons:string[] };
export type Analysis = { fund:Fund; initialAmount:number; monthlyAmount:number; totalContributions:number; p10:number; p50:number; p90:number; sampleCount:number; historyStart:string; historyEnd:string; maxDrawdown:number; recommendedYears:number; holdingSampleCount:number; positiveReturnRate:number; holdingCriteria:HoldingCriteria; dataAsOf:string; methodology:string; nisaNote:string; disclaimer:string };
export type History = { fund:Fund; points:PricePoint[]; dataAsOf:string };
type APIError = { error?:string };
// 前端只呼叫同網域的 Go API；資料庫連線資訊永遠不會進入瀏覽器。
async function request<T>(path:string,init?:RequestInit):Promise<T>{const response=await fetch(path,init);const body=await response.json() as T&APIError;if(!response.ok)throw new Error(body.error??"資料讀取失敗");return body}
export const api={
	findFunds:(query:string)=>request<Fund[]>(`/api/funds?q=${encodeURIComponent(query)}`),
	analyze:(fundId:string,initialAmount:number,monthlyAmount:number)=>request<Analysis>("/api/analysis",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({fundId,initialAmount,monthlyAmount})}),
	history:(fundId:string)=>request<History>(`/api/history/${encodeURIComponent(fundId)}`),
	insights:(fundId:string)=>request<Insight[]>(`/api/insights/${encodeURIComponent(fundId)}`),
	refresh:()=>request<{fundsUpdated:number;message:string}>("/api/data/refresh",{method:"POST"}),
};
