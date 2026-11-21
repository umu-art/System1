import { useEffect, useState } from 'react'
import { Configuration, DialogsApi, type ServiceInfoDto } from 's1-ui-api'

const dialogs = new DialogsApi(new Configuration({ basePath: '' }))

export default function App() {
  const [info, setInfo] = useState<ServiceInfoDto>()
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    dialogs.getDialogsInfo().then(setInfo, () => setFailed(true))
  }, [])

  return (
    <main>
      <h1>System1</h1>
      <p>{info ? `${info.service} · ${info.env}` : failed ? 'Ядро недоступно' : '…'}</p>
    </main>
  )
}
