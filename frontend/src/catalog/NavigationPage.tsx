import { useOutletContext } from 'react-router-dom'
import NavigationCatalog from './NavigationCatalog'
import type { CatalogContext } from './types'
export default function NavigationPage() {
  const { notify } = useOutletContext<CatalogContext>()
  return <NavigationCatalog notify={notify} />
}
