// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package i18n

// Codes for the host_self.manage requirement on McHarbor's own containers and host.
const (
	// ErrHostSelfContainerRequired refuses an exec, file write, lifecycle or config change
	// on a McHarbor container (this node, a peer or a helper) to a caller without host_self.manage.
	ErrHostSelfContainerRequired MsgCode = "err.host_self.container_required"
	// ErrHostSelfHostAccessRequired refuses creating a container on McHarbor's own
	// environment that would reach its host (Docker socket, /, /proc, /etc, privileged).
	ErrHostSelfHostAccessRequired MsgCode = "err.host_self.host_access_required"
	// ErrHostSelfStackRequired refuses a stack action that would stop, remove or recreate McHarbor.
	ErrHostSelfStackRequired MsgCode = "err.host_self.stack_required"
	// ErrHostSelfWorkflowRequired refuses saving a workflow with a node whose target is
	// statically McHarbor's own, since the run acts for the system and has no user to ask.
	ErrHostSelfWorkflowRequired MsgCode = "err.host_self.workflow_required"
	// ErrHostSelfWorkflowUnauthorized fails a run's node that reached McHarbor's own
	// container, stack or host when the workflow was last saved without host_self.manage.
	ErrHostSelfWorkflowUnauthorized MsgCode = "err.host_self.workflow_unauthorized"
)

// hostSelfMessages holds the six translations of the host_self codes; the permission key
// is quoted verbatim because it is what the operator has to ask for.
var hostSelfMessages = map[Lang]map[MsgCode]string{
	EN: {
		ErrHostSelfContainerRequired:    "This container belongs to McHarbor itself. Acting on it needs the host_self.manage permission as well as the container permission",
		ErrHostSelfHostAccessRequired:   "This container would reach the host McHarbor runs on (a host bind mount, a host or shared namespace, privileged mode, extra capabilities or devices, a relaxed security profile, or McHarbor's own data). Creating it on this environment needs the host_self.manage permission",
		ErrHostSelfStackRequired:        "This stack runs McHarbor itself. Stopping, removing or redeploying it needs the host_self.manage permission",
		ErrHostSelfWorkflowRequired:     "This workflow has a node that acts on McHarbor's own containers, stacks or host. Saving it needs the host_self.manage permission, because the workflow later runs for the system with nobody to ask",
		ErrHostSelfWorkflowUnauthorized: "This workflow was last saved by someone without the host_self.manage permission, so its nodes may not act on McHarbor's own containers, stacks or host. Someone holding host_self.manage has to save it again",
	},
	NL: {
		ErrHostSelfContainerRequired:    "Deze container hoort bij McHarbor zelf. Een actie erop vereist de permissie host_self.manage naast de containerpermissie",
		ErrHostSelfHostAccessRequired:   "Deze container zou de host bereiken waarop McHarbor draait (een host-bind-mount, een host- of gedeelde namespace, privileged-modus, extra capabilities of devices, een versoepeld beveiligingsprofiel, of de eigen data van McHarbor). Hem op deze omgeving aanmaken vereist de permissie host_self.manage",
		ErrHostSelfStackRequired:        "Deze stack draait McHarbor zelf. Stoppen, verwijderen of opnieuw uitrollen vereist de permissie host_self.manage",
		ErrHostSelfWorkflowRequired:     "Deze workflow heeft een node die op de eigen containers, stacks of host van McHarbor werkt. Opslaan vereist de permissie host_self.manage, omdat de workflow later voor het systeem draait zonder iemand om het te vragen",
		ErrHostSelfWorkflowUnauthorized: "Deze workflow is het laatst opgeslagen door iemand zonder de permissie host_self.manage, dus zijn nodes mogen niet op de eigen containers, stacks of host van McHarbor werken. Iemand met host_self.manage moet hem opnieuw opslaan",
	},
	DE: {
		ErrHostSelfContainerRequired:    "Dieser Container gehört zu McHarbor selbst. Eine Aktion darauf erfordert zusätzlich zur Containerberechtigung die Berechtigung host_self.manage",
		ErrHostSelfHostAccessRequired:   "Dieser Container würde den Host erreichen, auf dem McHarbor läuft (ein Host-Bind-Mount, ein Host- oder geteilter Namespace, privilegierter Modus, zusätzliche Capabilities oder Geräte, ein gelockertes Sicherheitsprofil oder McHarbors eigene Daten). Ihn in dieser Umgebung zu erstellen erfordert die Berechtigung host_self.manage",
		ErrHostSelfStackRequired:        "Dieser Stack betreibt McHarbor selbst. Stoppen, Entfernen oder erneutes Bereitstellen erfordert die Berechtigung host_self.manage",
		ErrHostSelfWorkflowRequired:     "Dieser Workflow hat einen Knoten, der auf McHarbors eigene Container, Stacks oder den Host wirkt. Das Speichern erfordert die Berechtigung host_self.manage, weil der Workflow später für das System läuft, ohne dass jemand gefragt werden kann",
		ErrHostSelfWorkflowUnauthorized: "Dieser Workflow wurde zuletzt von jemandem ohne die Berechtigung host_self.manage gespeichert, daher dürfen seine Knoten nicht auf McHarbors eigene Container, Stacks oder den Host wirken. Jemand mit host_self.manage muss ihn erneut speichern",
	},
	ES: {
		ErrHostSelfContainerRequired:    "Este contenedor pertenece al propio McHarbor. Actuar sobre él requiere el permiso host_self.manage además del permiso del contenedor",
		ErrHostSelfHostAccessRequired:   "Este contenedor alcanzaría el host donde se ejecuta McHarbor (un montaje bind del host, un namespace del host o compartido, modo privilegiado, capacidades o dispositivos adicionales, un perfil de seguridad relajado o los propios datos de McHarbor). Crearlo en este entorno requiere el permiso host_self.manage",
		ErrHostSelfStackRequired:        "Este stack ejecuta el propio McHarbor. Detenerlo, eliminarlo o volver a desplegarlo requiere el permiso host_self.manage",
		ErrHostSelfWorkflowRequired:     "Este flujo de trabajo tiene un nodo que actúa sobre los propios contenedores, stacks o el host de McHarbor. Guardarlo requiere el permiso host_self.manage, porque el flujo se ejecuta después para el sistema sin nadie a quien preguntar",
		ErrHostSelfWorkflowUnauthorized: "Este flujo de trabajo lo guardó por última vez alguien sin el permiso host_self.manage, así que sus nodos no pueden actuar sobre los propios contenedores, stacks o el host de McHarbor. Alguien con host_self.manage tiene que volver a guardarlo",
	},
	FR: {
		ErrHostSelfContainerRequired:    "Ce conteneur appartient à McHarbor lui-même. Agir dessus nécessite l'autorisation host_self.manage en plus de l'autorisation du conteneur",
		ErrHostSelfHostAccessRequired:   "Ce conteneur atteindrait l'hôte sur lequel tourne McHarbor (un montage bind de l'hôte, un namespace de l'hôte ou partagé, le mode privilégié, des capacités ou périphériques supplémentaires, un profil de sécurité assoupli, ou les données propres de McHarbor). Le créer dans cet environnement nécessite l'autorisation host_self.manage",
		ErrHostSelfStackRequired:        "Cette stack exécute McHarbor lui-même. L'arrêter, la supprimer ou la redéployer nécessite l'autorisation host_self.manage",
		ErrHostSelfWorkflowRequired:     "Ce flux de travail contient un nœud qui agit sur les conteneurs, stacks ou l'hôte propres à McHarbor. L'enregistrer nécessite l'autorisation host_self.manage, car le flux s'exécute ensuite pour le système sans personne à qui demander",
		ErrHostSelfWorkflowUnauthorized: "Ce flux de travail a été enregistré en dernier par quelqu'un sans l'autorisation host_self.manage, ses nœuds ne peuvent donc pas agir sur les conteneurs, stacks ou l'hôte propres à McHarbor. Une personne disposant de host_self.manage doit l'enregistrer à nouveau",
	},
	PT: {
		ErrHostSelfContainerRequired:    "Este contêiner pertence ao próprio McHarbor. Agir sobre ele exige a permissão host_self.manage além da permissão do contêiner",
		ErrHostSelfHostAccessRequired:   "Este contêiner alcançaria o host em que o McHarbor é executado (um bind mount do host, um namespace do host ou compartilhado, modo privilegiado, capacidades ou dispositivos extras, um perfil de segurança relaxado, ou os próprios dados do McHarbor). Criá-lo neste ambiente exige a permissão host_self.manage",
		ErrHostSelfStackRequired:        "Este stack executa o próprio McHarbor. Pará-lo, removê-lo ou reimplantá-lo exige a permissão host_self.manage",
		ErrHostSelfWorkflowRequired:     "Este fluxo de trabalho tem um nó que atua nos próprios contêineres, stacks ou host do McHarbor. Salvá-lo exige a permissão host_self.manage, porque o fluxo depois é executado para o sistema sem ninguém a quem perguntar",
		ErrHostSelfWorkflowUnauthorized: "Este fluxo de trabalho foi salvo por último por alguém sem a permissão host_self.manage, então seus nós não podem atuar nos próprios contêineres, stacks ou host do McHarbor. Alguém com host_self.manage precisa salvá-lo novamente",
	},
}

// init folds hostSelfMessages into the per-language maps T() reads, as codes_storage.go
// does, instead of growing the six maps in messages.go.
func init() {
	for lang, msgs := range hostSelfMessages {
		target, ok := translations[lang]
		if !ok {
			continue
		}
		for code, text := range msgs {
			target[code] = text
		}
	}
}
