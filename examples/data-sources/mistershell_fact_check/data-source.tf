data "mistershell_fact_check" "example" {
  id = "123"
}

# Alternatively, resolve one exact name.
data "mistershell_fact_check" "by_name" {
  name = "fact-check"
}
