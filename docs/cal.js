async function main() {
  eYear.value = new Date().getFullYear()

  let go = new Go()
  let binary = await fetch("cal.wasm")
  let wasm = await WebAssembly.instantiateStreaming(binary, go.importObject)
  window.year = 0
  await go.run(wasm.instance)

  eShowButton.onclick = async () => {
    let binary = await fetch("cal.wasm")
    let wasm = await WebAssembly.instantiateStreaming(binary, go.importObject)
    window.year = parseInt(eYear.value)
    await go.run(wasm.instance)
  }
}

main()
