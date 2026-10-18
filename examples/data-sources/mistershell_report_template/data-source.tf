data "mistershell_report_template" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_report_template" "by_name" {
  name = "report-template"
}
