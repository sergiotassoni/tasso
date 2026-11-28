// Package disinstalla contiene solo le risorse Windows di "Disinstalla Tasso.exe"
// (rsrc_windows_amd64.syso: come TassoSetup.exe, ma con il suo nome nelle Proprietà).
// Sta in un pacchetto a parte, salvato nel repository, perché "compila.sh disinstallatore" non
// debba più riscrivere le risorse dell'installatore: così anche il disinstallatore risulta
// compilato da codice pulito (vcs.modified=false, niente "+dirty").
package disinstalla
