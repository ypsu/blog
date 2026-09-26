import { error, iio } from "./iio.js"

declare var eSurveyForm: HTMLFormElement
declare var eSurveyID: HTMLElement
declare var eSubmitStatus: HTMLElement
declare var eSubmitSurvey: HTMLButtonElement

let surveyui = {
  init: async function (): Promise<error> {
    eSubmitSurvey.onclick = surveyui.submit
    eSubmitStatus.textContent = ''
    surveyui.surveyID = eSurveyID.textContent

    if (iio.User == "") {
      eSubmitSurvey.disabled = false
      return ""
    }

    eSubmitStatus.textContent = "Checking for a previous response..."
    let [rsp, err] = await iio.Fetch("/surveyapi?survey=" + surveyui.surveyID)
    if (err != "" && !err.includes("surveyapi.SurveyResponseNotFound")) {
      eSubmitStatus.textContent = "Error loading previous response: " + err
      return ""
    }
    eSubmitSurvey.disabled = false
    eSubmitStatus.textContent = ""
    if (rsp == "") return ""

    eSubmitSurvey.textContent = "Update"
    let data: Record<string, string> = JSON.parse(rsp)
    for (let [key, value] of Object.entries(data)) {
      let field = eSurveyForm.elements.namedItem(key)
      if (!(field instanceof HTMLTextAreaElement)) {
        console.log("surveyapi.BadResponseElement name=" + key)
        continue
      }
      field.value = value
    }
    return ""
  },

  submit: async function () {
    eSubmitSurvey.disabled = true

    let entries = Object.fromEntries(new FormData(eSurveyForm).entries())
    for (let qid in entries) {
      if (entries[qid].toString().trim() == "") delete entries[qid]
    }
    if (Object.keys(entries).length == 0) {
      eSubmitSurvey.disabled = false
      eSubmitStatus.textContent = "Please provide at least one answer before submitting."
      return
    }
    let data = JSON.stringify(entries)

    if (iio.User == "") {
      eSubmitStatus.textContent = "Registering..."
      let err = await iio.RegisterGuest()
      if (err != "") {
        eSubmitStatus.textContent = "Error registering: " + err + "; please try again tomorrow"
        eSubmitSurvey.disabled = false
        return
      }
    }

    eSubmitStatus.textContent = "Submitting..."
    let [rsp, err] = await iio.Fetch("/surveyapi?survey=" + surveyui.surveyID, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: data,
    })
    eSubmitSurvey.disabled = false
    if (err != "") {
      eSubmitStatus.textContent = "Error submitting: " + err
      return
    }
    eSubmitStatus.innerHTML = "<em>Thank you for the response.</em>"
    eSubmitSurvey.hidden = true
    eSurveyForm.hidden = true
  },

  surveyID: "",
}

iio.Run(surveyui.init)
