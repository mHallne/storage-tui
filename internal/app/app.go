package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"storage-tui/internal/azure"
)

type itemKind int

const (
	kindRoot itemKind = iota
	kindNone
	kindSubscription
	kindAccount
	kindContainer
	kindBlob
)

type pane int

const (
	paneAccounts pane = iota
	paneContents
	panePreview
)

type itemRef struct {
	Kind             itemKind
	Name             string
	SubscriptionID   string
	SubscriptionName string
	Account          string
	Container        string
	Region           string
	PublicAccess     string
	SizeBytes        int64
	Modified         time.Time
	ContentType      string
}

type App struct {
	provider            azure.Provider
	app                 *tview.Application
	pages               *tview.Pages
	accounts            *tview.TreeView
	contents            *tview.Table
	preview             *tview.TextView
	details             *tview.TextView
	searchForm          *tview.Form
	searchInput         *tview.InputField
	searchOpen          bool
	root                *tview.TreeNode
	rootRef             itemRef
	contentRefs         []itemRef
	activePane          pane
	loadingTree         bool
	loadingContents     bool
	lastDetails         string
	lastPreview         string
	previewFull         string
	previewSearch       string
	previewSearchable   bool
	subscriptionEnabled map[string]bool
}

func New(provider azure.Provider) *App {
	application := tview.NewApplication()
	pages := tview.NewPages()
	accounts := tview.NewTreeView()
	contents := tview.NewTable()
	preview := tview.NewTextView()
	details := tview.NewTextView()

	header := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetText("Azure Storage Explorer TUI  q: quit | r: refresh | tab: switch pane | enter/right: expand or collapse | left: parent or collapse | space: toggle subscription | /: search | esc: clear search")

	rootRef := itemRef{Kind: kindRoot, Name: "Subscriptions"}
	root := tview.NewTreeNode(rootRef.Name).
		SetReference(rootRef).
		SetSelectable(false).
		SetExpanded(true)

	accounts.SetRoot(root).SetCurrentNode(root)
	accounts.SetBorder(true).SetTitle("Subscriptions")
	contents.SetBorder(true).SetTitle("Contents")
	contents.SetSelectable(true, false)
	preview.SetBorder(true).SetTitle("Preview")
	preview.SetWrap(true)
	preview.SetWordWrap(true)
	preview.SetScrollable(true)
	details.SetBorder(true).SetTitle("Details")

	a := &App{
		provider:            provider,
		app:                 application,
		pages:               pages,
		accounts:            accounts,
		contents:            contents,
		preview:             preview,
		details:             details,
		root:                root,
		rootRef:             rootRef,
		activePane:          paneAccounts,
		contentRefs:         nil,
		subscriptionEnabled: make(map[string]bool),
	}

	a.accounts.SetChangedFunc(func(node *tview.TreeNode) {
		if a.loadingTree {
			return
		}
		a.onTreeChanged(node)
	})

	a.accounts.SetSelectedFunc(func(node *tview.TreeNode) {
		if a.loadingTree {
			return
		}
		a.onTreeSelected(node)
	})

	a.accounts.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRight:
			a.expandTreeNode(a.accounts.GetCurrentNode(), true)
			return nil
		case tcell.KeyLeft:
			node := a.accounts.GetCurrentNode()
			if node == nil {
				return nil
			}
			if node.IsExpanded() && len(node.GetChildren()) > 0 {
				node.SetExpanded(false)
				return nil
			}
			parent := a.parentNode(node)
			if parent != nil {
				a.accounts.SetCurrentNode(parent)
			}
			return nil
		case tcell.KeyRune:
			if event.Rune() == ' ' {
				if a.toggleSelectedSubscription() {
					return nil
				}
			}
		}
		return event
	})

	a.contents.SetSelectionChangedFunc(func(row, column int) {
		if a.loadingContents {
			return
		}
		a.onContentChanged(row)
	})

	a.contents.SetSelectedFunc(func(row, column int) {
		if a.loadingContents {
			return
		}
		a.onContentSelected(row)
	})

	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if a.searchOpen {
			if event.Key() == tcell.KeyCtrlC {
				a.app.Stop()
				return nil
			}
			return event
		}
		switch event.Key() {
		case tcell.KeyCtrlC:
			a.app.Stop()
			return nil
		case tcell.KeyTAB, tcell.KeyBacktab:
			a.cyclePane(event.Key() == tcell.KeyBacktab)
			return nil
		case tcell.KeyEsc:
			if a.activePane == panePreview && a.previewSearch != "" {
				a.clearSearch()
				return nil
			}
		}
		switch event.Rune() {
		case 'q':
			a.app.Stop()
			return nil
		case 'r':
			a.reload()
			return nil
		case '/':
			if a.activePane == panePreview && a.previewSearchable {
				a.openSearchModal()
				return nil
			}
		}
		return event
	})

	mainColumn := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(contents, 0, 2, true).
		AddItem(preview, 0, 1, false)

	body := tview.NewFlex().
		AddItem(accounts, 0, 1, true).
		AddItem(mainColumn, 0, 3, false)

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(details, 7, 0, false)

	a.pages.AddPage("main", layout, true, true)
	a.setupSearchModal()
	a.app.SetRoot(a.pages, true).SetFocus(accounts)
	a.reload()

	return a
}

func (a *App) Run() error {
	return a.app.Run()
}

func (a *App) reload() {
	if err := a.loadSubscriptions(); err != nil {
		a.showSubscriptionsError(err)
		return
	}
	a.showEmptyContents("Select a container to view blobs.")
	a.refreshDetails()
}

func (a *App) setupSearchModal() {
	input := tview.NewInputField().
		SetLabel("Find: ").
		SetFieldWidth(0)
	form := tview.NewForm().
		AddFormItem(input).
		AddButton("Search", func() {
			a.applySearch(input.GetText())
			a.closeSearchModal()
		}).
		AddButton("Cancel", func() {
			a.clearSearch()
			a.closeSearchModal()
		})
	form.SetBorder(true).SetTitle("Search Preview")
	form.SetButtonsAlign(tview.AlignRight)

	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.applySearch(input.GetText())
			a.closeSearchModal()
		case tcell.KeyEsc:
			a.clearSearch()
			a.closeSearchModal()
		}
	})

	a.searchInput = input
	a.searchForm = form
	a.pages.AddPage("search", centerModal(form, 7, 60), true, false)
}

func (a *App) openSearchModal() {
	a.searchOpen = true
	a.searchInput.SetText(a.previewSearch)
	a.pages.ShowPage("search")
	a.app.SetFocus(a.searchInput)
}

func (a *App) closeSearchModal() {
	a.pages.HidePage("search")
	a.searchOpen = false
	a.setActivePane(panePreview)
}

func (a *App) togglePane() {
	if a.activePane == paneAccounts {
		a.setActivePane(paneContents)
	} else {
		a.setActivePane(paneAccounts)
	}
}

func (a *App) cyclePane(reverse bool) {
	var order []pane
	if reverse {
		order = []pane{panePreview, paneContents, paneAccounts}
	} else {
		order = []pane{paneAccounts, paneContents, panePreview}
	}
	for i, paneID := range order {
		if paneID == a.activePane {
			next := order[(i+1)%len(order)]
			a.setActivePane(next)
			return
		}
	}
	a.setActivePane(paneAccounts)
}

func (a *App) setActivePane(target pane) {
	a.activePane = target
	if target == paneAccounts {
		a.app.SetFocus(a.accounts)
	} else if target == paneContents {
		a.app.SetFocus(a.contents)
	} else {
		a.app.SetFocus(a.preview)
	}
	a.refreshDetails()
}

func (a *App) loadSubscriptions() error {
	ctx := context.Background()
	subscriptions, err := a.provider.ListSubscriptions(ctx)
	if err != nil {
		return err
	}

	a.loadingTree = true
	a.root.ClearChildren()
	a.root.SetExpanded(true)
	a.accounts.SetTitle("Subscriptions")
	a.subscriptionEnabled = a.mergeSubscriptionSelections(subscriptions)

	if len(subscriptions) == 0 {
		ref := itemRef{Kind: kindNone, Name: "No subscriptions found."}
		node := tview.NewTreeNode(ref.Name).SetReference(ref).SetSelectable(true)
		a.root.AddChild(node)
		a.accounts.SetCurrentNode(node)
		a.loadingTree = false
		return nil
	}

	for _, subscription := range subscriptions {
		enabled := a.isSubscriptionEnabled(subscription.ID)
		ref := itemRef{
			Kind:             kindSubscription,
			Name:             subscription.Name,
			SubscriptionID:   subscription.ID,
			SubscriptionName: subscription.Name,
		}
		node := tview.NewTreeNode(subscriptionLabel(ref.Name, enabled)).SetReference(ref).SetSelectable(true)
		if enabled {
			if err := a.loadAccountsForSubscription(node, ref); err != nil {
				return err
			}
			node.SetExpanded(true)
		}
		a.root.AddChild(node)
	}

	children := a.root.GetChildren()
	if len(children) > 0 {
		a.accounts.SetCurrentNode(children[0])
	} else {
		a.accounts.SetCurrentNode(a.root)
	}
	a.loadingTree = false

	return nil
}

func (a *App) mergeSubscriptionSelections(subscriptions []azure.Subscription) map[string]bool {
	next := make(map[string]bool, len(subscriptions))
	for _, subscription := range subscriptions {
		enabled, ok := a.subscriptionEnabled[subscription.ID]
		if !ok {
			enabled = true
		}
		next[subscription.ID] = enabled
	}
	return next
}

func subscriptionLabel(name string, enabled bool) string {
	if enabled {
		return fmt.Sprintf("(x) %s", name)
	}
	return fmt.Sprintf("( ) %s", name)
}

func (a *App) isSubscriptionEnabled(subscriptionID string) bool {
	if subscriptionID == "" {
		return true
	}
	enabled, ok := a.subscriptionEnabled[subscriptionID]
	if !ok {
		return true
	}
	return enabled
}

func (a *App) toggleSelectedSubscription() bool {
	node := a.accounts.GetCurrentNode()
	if node == nil {
		return false
	}
	ref, ok := node.GetReference().(itemRef)
	if !ok || ref.Kind != kindSubscription {
		return false
	}
	a.toggleSubscription(node, ref)
	return true
}

func (a *App) toggleSubscription(node *tview.TreeNode, subscription itemRef) {
	enabled := a.isSubscriptionEnabled(subscription.SubscriptionID)
	enabled = !enabled
	a.subscriptionEnabled[subscription.SubscriptionID] = enabled
	node.SetText(subscriptionLabel(subscription.Name, enabled))

	if !enabled {
		node.ClearChildren()
		node.SetExpanded(false)
		if a.activePane == paneAccounts {
			a.updateDetails(subscription)
		}
		a.showEmptyContents("Subscription disabled.")
		return
	}

	if err := a.loadAccountsForSubscription(node, subscription); err != nil {
		a.showTreeLoadError("accounts", err)
		errorRef := itemRef{
			Kind:             kindNone,
			Name:             "Error loading accounts.",
			SubscriptionID:   subscription.SubscriptionID,
			SubscriptionName: subscription.SubscriptionName,
		}
		node.ClearChildren()
		node.AddChild(tview.NewTreeNode(errorRef.Name).SetReference(errorRef).SetSelectable(true))
		return
	}

	node.SetExpanded(true)
	if a.activePane == paneAccounts {
		a.updateDetails(subscription)
	}
	a.showEmptyContents("Select an account to view containers.")
}

func (a *App) loadAccountsForSubscription(node *tview.TreeNode, subscription itemRef) error {
	ctx := context.Background()
	accounts, err := a.provider.ListAccounts(ctx, subscription.SubscriptionID)
	if err != nil {
		return err
	}

	node.ClearChildren()
	if len(accounts) == 0 {
		ref := itemRef{
			Kind:             kindNone,
			Name:             "No accounts.",
			SubscriptionID:   subscription.SubscriptionID,
			SubscriptionName: subscription.SubscriptionName,
		}
		child := tview.NewTreeNode(ref.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
		return nil
	}

	for _, account := range accounts {
		ref := itemRef{
			Kind:             kindAccount,
			Name:             account.Name,
			SubscriptionID:   subscription.SubscriptionID,
			SubscriptionName: subscription.SubscriptionName,
			Account:          account.Name,
			Region:           account.Region,
		}
		child := tview.NewTreeNode(account.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
	}

	return nil
}

func (a *App) loadContainers(node *tview.TreeNode, account itemRef) error {
	ctx := context.Background()
	containers, err := a.provider.ListContainers(ctx, account.Account)
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		ref := itemRef{
			Kind:             kindNone,
			Name:             "No containers.",
			SubscriptionID:   account.SubscriptionID,
			SubscriptionName: account.SubscriptionName,
			Account:          account.Account,
		}
		child := tview.NewTreeNode(ref.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
		return nil
	}

	for _, container := range containers {
		ref := itemRef{
			Kind:             kindContainer,
			Name:             container.Name,
			SubscriptionID:   account.SubscriptionID,
			SubscriptionName: account.SubscriptionName,
			Account:          account.Account,
			Container:        container.Name,
			PublicAccess:     container.PublicAccess,
		}
		child := tview.NewTreeNode(container.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
	}

	return nil
}

func (a *App) loadBlobChildren(node *tview.TreeNode, container itemRef) error {
	ctx := context.Background()
	blobs, err := a.provider.ListBlobs(ctx, container.Account, container.Container)
	if err != nil {
		return err
	}

	if len(blobs) == 0 {
		ref := itemRef{
			Kind:             kindNone,
			Name:             "No blobs.",
			SubscriptionID:   container.SubscriptionID,
			SubscriptionName: container.SubscriptionName,
			Account:          container.Account,
			Container:        container.Container,
		}
		child := tview.NewTreeNode(ref.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
		return nil
	}

	for _, blob := range blobs {
		ref := itemRef{
			Kind:             kindBlob,
			Name:             blob.Name,
			SubscriptionID:   container.SubscriptionID,
			SubscriptionName: container.SubscriptionName,
			Account:          container.Account,
			Container:        container.Container,
			SizeBytes:        blob.SizeBytes,
			Modified:         blob.Modified,
			ContentType:      blob.ContentType,
		}
		child := tview.NewTreeNode(blob.Name).SetReference(ref).SetSelectable(true)
		node.AddChild(child)
	}

	return nil
}

func (a *App) expandTreeNode(node *tview.TreeNode, toggle bool) {
	if node == nil {
		return
	}
	ref, ok := node.GetReference().(itemRef)
	if !ok {
		return
	}

	switch ref.Kind {
	case kindSubscription:
		if !a.isSubscriptionEnabled(ref.SubscriptionID) {
			return
		}
		if len(node.GetChildren()) == 0 {
			if err := a.loadAccountsForSubscription(node, ref); err != nil {
				a.showTreeLoadError("accounts", err)
				return
			}
		}
	case kindAccount:
		if len(node.GetChildren()) == 0 {
			if err := a.loadContainers(node, ref); err != nil {
				a.showTreeLoadError("containers", err)
				return
			}
		}
	case kindContainer:
		if len(node.GetChildren()) == 0 {
			if err := a.loadBlobChildren(node, ref); err != nil {
				a.showTreeLoadError("blobs", err)
				return
			}
		}
	default:
		return
	}

	if toggle {
		node.SetExpanded(!node.IsExpanded())
	} else {
		node.SetExpanded(true)
	}
}

func (a *App) parentNode(target *tview.TreeNode) *tview.TreeNode {
	if target == nil || target == a.root || a.root == nil {
		return nil
	}
	return a.findParentNode(target, a.root, nil)
}

func (a *App) findParentNode(target, node, parent *tview.TreeNode) *tview.TreeNode {
	if node == target {
		return parent
	}
	for _, child := range node.GetChildren() {
		if found := a.findParentNode(target, child, node); found != nil {
			return found
		}
	}
	return nil
}

func (a *App) onTreeChanged(node *tview.TreeNode) {
	if node == nil {
		return
	}
	ref, ok := node.GetReference().(itemRef)
	if !ok {
		return
	}

	switch ref.Kind {
	case kindContainer:
		if err := a.showBlobs(ref); err != nil {
			a.showLoadError("blobs", err)
			return
		}
	case kindBlob:
		a.updatePreview(ref)
	case kindAccount:
		a.showEmptyContents("Select a container to view blobs.")
	case kindSubscription:
		if a.isSubscriptionEnabled(ref.SubscriptionID) {
			a.showEmptyContents("Select an account to view containers.")
		} else {
			a.showEmptyContents("Subscription disabled.")
		}
	case kindRoot:
		a.showEmptyContents("Select a subscription to view accounts.")
	case kindNone:
		a.showEmptyContents(ref.Name)
	}

	if a.activePane == paneAccounts {
		a.updateDetails(ref)
	}
}

func (a *App) onTreeSelected(node *tview.TreeNode) {
	if node == nil {
		return
	}
	ref, ok := node.GetReference().(itemRef)
	if !ok {
		return
	}

	switch ref.Kind {
	case kindSubscription:
		a.expandTreeNode(node, true)
	case kindAccount:
		a.expandTreeNode(node, true)
	case kindContainer:
		a.expandTreeNode(node, true)
	}
}

func (a *App) addContentRow(ref itemRef, name, details string) {
	row := len(a.contentRefs)
	a.contentRefs = append(a.contentRefs, ref)
	nameCell := tview.NewTableCell(name).SetExpansion(1)
	detailCell := tview.NewTableCell(details).SetAlign(tview.AlignRight)
	a.contents.SetCell(row, 0, nameCell)
	a.contents.SetCell(row, 1, detailCell)
}

func (a *App) showBlobs(container itemRef) error {
	ctx := context.Background()
	blobs, err := a.provider.ListBlobs(ctx, container.Account, container.Container)
	if err != nil {
		return err
	}

	a.loadingContents = true
	a.contents.Clear()
	a.contentRefs = nil

	for _, blob := range blobs {
		ref := itemRef{
			Kind:             kindBlob,
			Name:             blob.Name,
			SubscriptionID:   container.SubscriptionID,
			SubscriptionName: container.SubscriptionName,
			Account:          container.Account,
			Container:        container.Container,
			SizeBytes:        blob.SizeBytes,
			Modified:         blob.Modified,
			ContentType:      blob.ContentType,
		}
		a.addContentRow(ref, blob.Name, formatContentDetails(ref))
	}

	if len(blobs) == 0 {
		ref := itemRef{
			Kind:             kindNone,
			Name:             "No blobs in container.",
			SubscriptionID:   container.SubscriptionID,
			SubscriptionName: container.SubscriptionName,
			Account:          container.Account,
			Container:        container.Container,
		}
		a.addContentRow(ref, ref.Name, "")
	}

	a.contents.Select(0, 0)
	a.loadingContents = false
	a.contents.SetTitle(fmt.Sprintf("Contents: %s/%s", container.Account, container.Name))
	a.setPreviewContent("Select a blob to preview.", false)
	a.refreshContentSelection()

	return nil
}

func (a *App) onContentChanged(row int) {
	ref, ok := a.contentRef(row)
	if !ok {
		return
	}
	a.updatePreview(ref)
	if a.activePane == paneContents || a.activePane == panePreview {
		a.updateDetails(ref)
	}
}

func (a *App) onContentSelected(row int) {
	ref, ok := a.contentRef(row)
	if !ok {
		return
	}

	if ref.Kind == kindBlob {
		a.setActivePane(paneContents)
	}
}

func (a *App) refreshContentSelection() {
	row, _ := a.contents.GetSelection()
	a.onContentChanged(row)
}

func (a *App) contentRef(row int) (itemRef, bool) {
	if row < 0 || row >= len(a.contentRefs) {
		return itemRef{}, false
	}
	return a.contentRefs[row], true
}

func (a *App) refreshDetails() {
	if a.activePane == paneAccounts {
		a.onTreeChanged(a.accounts.GetCurrentNode())
		return
	}

	row, _ := a.contents.GetSelection()
	ref, ok := a.contentRef(row)
	if !ok {
		a.setDetailsText("No selection.")
		return
	}
	a.updateDetails(ref)
}

func (a *App) showEmptyContents(message string) {
	a.loadingContents = true
	a.contents.Clear()
	a.contentRefs = nil
	ref := itemRef{Kind: kindNone, Name: message}
	a.addContentRow(ref, ref.Name, "")
	a.contents.Select(0, 0)
	a.loadingContents = false
	a.contents.SetTitle("Contents")
	a.setPreviewContent("Select a blob to preview.", false)
}

func (a *App) showLoadError(scope string, err error) {
	message := fmt.Sprintf("Error loading %s: %v", scope, err)
	a.setDetailsText(message)
	a.setPreviewContent("Unable to load data.", false)

	a.loadingContents = true
	a.contents.Clear()
	a.contentRefs = nil
	ref := itemRef{Kind: kindNone, Name: "Error loading data."}
	a.addContentRow(ref, ref.Name, "")
	a.contents.Select(0, 0)
	a.loadingContents = false
	a.contents.SetTitle("Contents")
}

func (a *App) showSubscriptionsError(err error) {
	a.loadingTree = true
	a.root.ClearChildren()
	ref := itemRef{Kind: kindNone, Name: "Error loading subscriptions."}
	node := tview.NewTreeNode(ref.Name).SetReference(ref).SetSelectable(true)
	a.root.AddChild(node)
	a.accounts.SetCurrentNode(node)
	a.loadingTree = false
	a.showTreeLoadError("subscriptions", err)
}

func (a *App) showTreeLoadError(scope string, err error) {
	message := fmt.Sprintf("Error loading %s: %v", scope, err)
	a.setDetailsText(message)
	a.setPreviewContent("Unable to load data.", false)
}

func (a *App) updateDetails(ref itemRef) {
	var text string
	switch ref.Kind {
	case kindRoot:
		text = "Select a subscription to browse accounts."
	case kindNone:
		text = ref.Name
	case kindSubscription:
		status := "enabled"
		if !a.isSubscriptionEnabled(ref.SubscriptionID) {
			status = "disabled"
		}
		text = fmt.Sprintf("Subscription: %s\nStatus: %s", ref.Name, status)
	case kindAccount:
		lines := []string{fmt.Sprintf("Account: %s", ref.Name)}
		if ref.SubscriptionName != "" {
			lines = append(lines, fmt.Sprintf("Subscription: %s", ref.SubscriptionName))
		}
		if ref.Region != "" {
			lines = append(lines, fmt.Sprintf("Region: %s", ref.Region))
		}
		text = strings.Join(lines, "\n")
	case kindContainer:
		lines := []string{
			fmt.Sprintf("Container: %s", ref.Name),
			fmt.Sprintf("Account: %s", ref.Account),
		}
		if ref.SubscriptionName != "" {
			lines = append(lines, fmt.Sprintf("Subscription: %s", ref.SubscriptionName))
		}
		if ref.PublicAccess != "" {
			lines = append(lines, fmt.Sprintf("Public access: %s", ref.PublicAccess))
		}
		text = strings.Join(lines, "\n")
	case kindBlob:
		lines := []string{
			fmt.Sprintf("Blob: %s", ref.Name),
			fmt.Sprintf("Account: %s", ref.Account),
			fmt.Sprintf("Container: %s", ref.Container),
		}
		if ref.SubscriptionName != "" {
			lines = append(lines, fmt.Sprintf("Subscription: %s", ref.SubscriptionName))
		}
		lines = append(lines, fmt.Sprintf("Size: %s", formatBytes(ref.SizeBytes)))
		lines = append(lines, fmt.Sprintf("Modified: %s", formatTime(ref.Modified)))
		if ref.ContentType != "" {
			lines = append(lines, fmt.Sprintf("Content type: %s", ref.ContentType))
		}
		text = strings.Join(lines, "\n")
	default:
		text = "No selection."
	}

	a.setDetailsText(text)
}

func (a *App) updatePreview(ref itemRef) {
	var text string
	searchable := false
	switch ref.Kind {
	case kindBlob:
		text = previewForBlob(ref)
		searchable = true
	case kindNone:
		text = "No preview available."
	default:
		text = "Select a blob to preview."
	}
	a.setPreviewContent(text, searchable)
}

func formatContentDetails(ref itemRef) string {
	if ref.Kind != kindBlob {
		return ""
	}
	return fmt.Sprintf("%s | %s | %s", ref.ContentType, formatBytes(ref.SizeBytes), formatTime(ref.Modified))
}

func (a *App) setDetailsText(text string) {
	if text == a.lastDetails {
		return
	}
	a.details.SetText(text)
	a.lastDetails = text
}

func (a *App) setPreviewText(text string) {
	if text == a.lastPreview {
		return
	}
	a.preview.SetText(text)
	a.lastPreview = text
}

func (a *App) setPreviewContent(text string, searchable bool) {
	a.previewFull = text
	a.previewSearchable = searchable
	a.applyPreviewFilter()
}

func (a *App) applySearch(term string) {
	trimmed := strings.TrimSpace(term)
	if trimmed == "" {
		a.clearSearch()
		return
	}
	a.previewSearch = trimmed
	a.applyPreviewFilter()
}

func (a *App) clearSearch() {
	if a.previewSearch == "" {
		return
	}
	a.previewSearch = ""
	a.applyPreviewFilter()
}

func (a *App) applyPreviewFilter() {
	display := a.previewFull
	if a.previewSearchable && a.previewSearch != "" {
		display = filterPreviewText(a.previewFull, a.previewSearch)
	}
	a.setPreviewText(display)
	a.updatePreviewTitle()
}

func (a *App) updatePreviewTitle() {
	title := "Preview"
	if a.previewSearchable && a.previewSearch != "" {
		title = fmt.Sprintf("Preview (filter: %s)", a.previewSearch)
	}
	a.preview.SetTitle(title)
}

func filterPreviewText(full, term string) string {
	if term == "" {
		return full
	}
	lines := strings.Split(full, "\n")
	headerEnd := -1
	for i, line := range lines {
		if line == "" {
			headerEnd = i
			break
		}
	}

	start := 0
	if headerEnd != -1 {
		start = headerEnd + 1
	}

	lowerTerm := strings.ToLower(term)
	var matches []string
	for _, line := range lines[start:] {
		if strings.Contains(strings.ToLower(line), lowerTerm) {
			matches = append(matches, line)
		}
	}

	var builder strings.Builder
	if headerEnd != -1 {
		builder.WriteString(strings.Join(lines[:headerEnd+1], "\n"))
		builder.WriteString("\n")
	}
	if len(matches) == 0 {
		builder.WriteString(fmt.Sprintf("No matches for %q.\n", term))
	} else {
		builder.WriteString(strings.Join(matches, "\n"))
		if !strings.HasSuffix(builder.String(), "\n") {
			builder.WriteString("\n")
		}
	}

	return builder.String()
}

func centerModal(primitive tview.Primitive, height, width int) tview.Primitive {
	row := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(nil, 0, 1, false).
		AddItem(primitive, height, 0, true).
		AddItem(nil, 0, 1, false)

	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(row, width, 0, true).
		AddItem(nil, 0, 1, false)
}

func previewForBlob(ref itemRef) string {
	header := fmt.Sprintf("File: %s\nContent-Type: %s\nSize: %s\n\n", ref.Name, ref.ContentType, formatBytes(ref.SizeBytes))
	switch ref.ContentType {
	case "text/plain":
		return header + sampleTextPreview(ref.Name)
	case "text/html":
		return header + sampleHTMLPreview(ref.Name)
	case "image/jpeg", "image/svg+xml":
		return header + "Binary content preview not available."
	default:
		return header + "Preview not available for this content type."
	}
}

func sampleTextPreview(name string) string {
	switch {
	case strings.HasSuffix(name, ".log"):
		return "2024-05-11T03:12:00Z INFO job=ingest msg=\"started\"\n2024-05-11T03:12:02Z INFO job=ingest msg=\"completed\"\n"
	case name == "robots.txt":
		return "User-agent: *\nDisallow: /private\n"
	default:
		return fmt.Sprintf("Preview placeholder for %s.\n", name)
	}
}

func sampleHTMLPreview(name string) string {
	if name == "index.html" {
		return "<!doctype html>\n<html>\n  <head>\n    <title>Storage Preview</title>\n  </head>\n  <body>\n    <h1>Hello from storage-tui</h1>\n    <p>This is mock HTML content.</p>\n  </body>\n</html>\n"
	}
	return "<!doctype html>\n<html>\n  <body>\n    <p>Mock HTML preview.</p>\n  </body>\n</html>\n"
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return "n/a"
	}
	return value.UTC().Format(time.RFC3339)
}

func formatBytes(value int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)

	switch {
	case value >= gb:
		return fmt.Sprintf("%.2f GB", float64(value)/float64(gb))
	case value >= mb:
		return fmt.Sprintf("%.2f MB", float64(value)/float64(mb))
	case value >= kb:
		return fmt.Sprintf("%.2f KB", float64(value)/float64(kb))
	default:
		return fmt.Sprintf("%d B", value)
	}
}
