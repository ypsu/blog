let reaction = ''

let demohtml = `
  <style>
  #hdemo {
    padding: 0.3em;
  }

  #hreactionbox {
    border: 1px solid black;
    padding: 0.2em;
  }

  .cflag {
    visibility: hidden;
  }

  :hover > .cflag {
    visibility: visible;
  }

  #hfeedback {
    display: none;
  }
  #hfeedback.cvisible {
    display: block;
  }

  #hhelp {
    display: none;
  }
  #hhelp.cvisible {
    display: block;
  }
  </style>

  <p>someuser at 2024-05-30 12:34 UTC:</p>
  <blockquote>some witty short motivational message that people can react to.</blockquote>
  <p style='cursor:default'><span id=hscore>+13</span> <span id=hreactionbox><span id=hreactionemoji onclick='defaultreact()'>👍</span> <span onclick=togglefeedback()>⋯</span></span><span id=hsummary>    👍19    🙏6    👎5    ♻️7</span>    <span onclick=togglefeedback()>⋯</span></p>

  <blockquote id=hfeedback style='cursor:default'>
    <div>
      <p>select your reaction:</p>
      <table id=hemojiselector>
      <tr>
      <td>upvote:
        <td id=hlike class=cselection onclick="setreaction('like')">👍(like)
        <td id=hthanks class=cselection onclick="setreaction('thanks')">🙏(thanks)
        <td id=hhug class=cselection onclick="setreaction('hug')">🫂(hug)
      <tr>
      <td>downvote:
        <td id=hdislike class=cselection onclick="setreaction('dislike')">👎(dislike)
        <td id=hduplicate class=cselection onclick="setreaction('duplicate')">♻️(duplicate)
        <td id=hinaccurate class=cselection onclick="setreaction('inaccurate')">🤨(inaccurate)
      <tr>
      <td>remove:
        <td id=hirrelevant class=cselection onclick="setreaction('irrelevant')">🗑️(irrelevant)
        <td id=hinappropriate class=cselection onclick="setreaction('inappropriate')">⛔(inappropriate)
        <td id=hsensitive class=cselection onclick="setreaction('sensitive')">🔒(sensitive)
      </table>
      <p>comment: <input id=hcomment size=30 maxlength=120 onkeyup=render()></p>
      <button onclick="hhelp.classList.toggle('cvisible')" href=#>help</button>
      <blockquote id=hhelp>this is where a more detailed description of the reactions could appear.</blockquote>
    </div>

    <div id=hreactions></div>

    see all reaction comments at <a onclick='alert("clicking this would show all comments on a separate page. this is not implemented in this demo.")' href=#>example.com/allcomments?id=123</a>.
  </blockquote>
`

function togglefeedback() {
  hfeedback.classList.toggle('cvisible')
}

function setreaction(newreaction) {
  if (newreaction == reaction) {
    reaction = ''
  } else {
    reaction = newreaction
  }
  render()
}

function defaultreact() {
  if (reaction == '') {
    setreaction('like')
  } else {
    setreaction('')
  }
}

function render() {
  for (let elem of document.querySelectorAll('#hemojiselector .cselection')) elem.classList.remove('cbgNotice')
  switch (reaction) {
    case '':
      hreactionemoji.innerText = '👍'
      hreactionbox.className = ''
      break
    case 'like':
      hreactionemoji.innerText = '👍'
      hreactionbox.className = 'cbgPositive'
      hlike.classList.add('cbgNotice')
      break
    case 'thanks':
      hreactionemoji.innerText = '🙏'
      hreactionbox.className = 'cbgPositive'
      hthanks.classList.add('cbgNotice')
      break
    case 'hug':
      hreactionemoji.innerText = '🫂'
      hreactionbox.className = 'cbgPositive'
      hhug.classList.add('cbgNotice')
      break
    case 'dislike':
      hreactionemoji.innerText = '👎'
      hreactionbox.className = 'cbgNegative'
      hdislike.classList.add('cbgNotice')
      break
    case 'duplicate':
      hreactionemoji.innerText = '♻️'
      hreactionbox.className = 'cbgNegative'
      hduplicate.classList.add('cbgNotice')
      break
    case 'inaccurate':
      hreactionemoji.innerText = '🤨'
      hreactionbox.className = 'cbgNegative'
      hinaccurate.classList.add('cbgNotice')
      break
    case 'irrelevant':
      hreactionemoji.innerText = '🗑️'
      hreactionbox.className = 'cbgSpecial'
      hirrelevant.classList.add('cbgNotice')
      break
    case 'inappropriate':
      hreactionemoji.innerText = '⛔'
      hreactionbox.className = 'cbgSpecial'
      hinappropriate.classList.add('cbgNotice')
      break
    case 'sensitive':
      hreactionemoji.innerText = '🔒'
      hreactionbox.className = 'cbgSpecial'
      hsensitive.classList.add('cbgNotice')
      break
  }

  let score = 13
  if (['like', 'thanks', 'hug'].indexOf(reaction) >= 0) score++
  if (['dislike', 'duplicate', 'inaccurate'].indexOf(reaction) >= 0) score--
  hscore.innerText = `+${score}`

  let summaryh = ''
  let h = ''
  let commenthtml = `<li>${escapehtml(hcomment.value)}`
  let flaghtml = `<span class=cflag title="report as inappropriate" onclick='alert("this is for flagging a comment for moderators. it would ask for a reason for flagging it.")'>🚩</span>`

  let likecnt = 19
  let likecomments = 10
  let likecomment = ''
  if (reaction == 'like') likecnt++
  if (reaction == 'like' && hcomment.value != '') likecomments++
  if (reaction == 'like' && hcomment.value != '') likecomment = commenthtml
  summaryh += `    <span title="${likecnt} like reactions">👍${likecnt}</span>`
  h += `
    <p>${likecnt} 👍(like) reactions. ${likecomments} comments, random sample:</p>
    <ul>
      ${likecomment}
      <li>great observation, keep it up! ${flaghtml}
      <li>this is why i'm following this channel! ${flaghtml}
      <li>most underappreciated thinker of our times! ${flaghtml}
      <li>this should be seen by everyone!!!1! ${flaghtml}
    </ul>
`

  let reactions = ['thanks', 'hug', 'dislike', 'duplicate', 'inaccurate', 'irrelevant', 'inappropriate', 'sensitive']
  let emojis = ['🙏', '🫂', '👎', '♻️', '🤨', '🗑️', '⛔', '🔒']
  let counts = [6, 0, 5, 7, 0, 0, 0, 0]
  let comments = {
    'thanks': [
      "this finally made me to commit to stop smoking!",
      "i had a bad day but this cheered me up, thanks mate!",
    ],
    'dislike': [
      "preach!",
      "you have nothing better to do than spread this nonsense?",
    ],
    'duplicate': [
      "this is a clear ripoff of someotheruser's content.",
    ],
  }
  for (let i = 0; i < 8; i++) {
    let count = counts[i]
    if (reactions[i] == reaction) count++
    if (count == 0) continue
    summaryh += `    <span title="${count} ${reactions[i]} reactions">${emojis[i]}${count}<span>`
    let commentcount = 0
    if (reactions[i] in comments) commentcount += comments[reactions[i]].length
    if (reactions[i] == reaction && hcomment.value != '') commentcount++
    h += `<p>${count} ${emojis[i]}(${reactions[i]}) reactions.`
    if (commentcount == 0) {
      h += `</p>\n`
      continue
    }
    h += ` ${commentcount} comments:\n<ul>`
    if (reactions[i] == reaction && hcomment.value != '') h += commenthtml
    if (reactions[i] in comments) {
      for (let c of comments[reactions[i]]) h += `<li>${escapehtml(c)} ${flaghtml}\n`
    }
    h += `</ul>\n\n`
  }

  hsummary.innerHTML = summaryh
  hreactions.innerHTML = h
}

function escapehtml(unsafe) {
  return unsafe.replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#039;');
}

function main() {
  hdemo.innerHTML = demohtml
  render()
}

main()
