fetch('gittime').then(resp => resp.text()).then(text => hgittime.innerText = text)
