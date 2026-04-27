// Package scoring, müşteri/AP/kule sinyal skorlarını üreten
// deterministik kural motorudur. ML kullanılmaz; eşikler ve sınıflandırma
// kuralları açıkça yazılır ve veritabanından override edilebilir.
//
// Faz 6 mimari özet:
//   - types.go: paylaşılan tipler (Inputs, Result, Diagnosis, Severity, Action)
//   - thresholds.go: varsayılan eşikler + DB override path
//   - diagnosis.go: tanı sınıflandırıcı
//   - actions.go: tanı → önerilen aksiyon eşleme
//   - ap_degradation.go: AP genelinde anomalileri tespit eden peer-group analizi
//   - trend.go: 7 günlük sinyal trend hesabı
//   - engine.go: ana skor motoru (Score / ScoreBatch / ScoreAP / ScoreTower)
//   - repository.go: pgx tabanlı kalıcı katman
//
// Bu dosya geriye dönük uyumluluk için Score() fonksiyonunun
// eski imzasını korur ve dahili olarak Engine.ScoreCustomer'a delege eder.
package scoring

// Score, eski imzalı yardımcıdır. Faz 6 ile birlikte tercih edilen yol
// Engine.ScoreCustomer kullanmaktır; bu fonksiyon DefaultThresholds() ile
// çalışan basit bir adapterdır ve geriye dönük uyumluluk amacıyla
// tutulmuştur.
func Score(in Inputs) Result {
	eng := NewEngine(DefaultThresholds())
	return eng.ScoreCustomer(in)
}
