package cpy_test

import (
	"fmt"
	"testing"

	"github.com/suisrc/zgg/ze/cpy"
)

type User0 struct {
	Name string
	Role string
	Age  int32

	// Explicitly ignored in the destination struct.
	Salary int
}

func (user *User0) DoubleAge() int32 {
	return 2 * user.Age
}

// Tags in the destination Struct provide instructions to copier.Copy to ignore
// or enforce copying and to panic or return an error if a field was not copied.
type Employee0 struct {
	// Tell cpy.Copy to panic if this field is not copied.
	Name string `cpy:"must"`

	// Tell cpy.Copy to return an error if this field is not copied.
	Age int32 `cpy:"must,nopanic"`

	// Tell cpy.Copy to explicitly ignore copying this field.
	Salary int `cpy:"-"`

	DoubleAge int32
	EmployeId int64
	SuperRole string
}

func (employee *Employee0) Role(role string) {
	employee.SuperRole = "Super " + role
}

func Test_copy(t *testing.T) {
	var (
		user      = User0{Name: "Jinzhu", Age: 18, Role: "Admin", Salary: 200000}
		users     = []User0{{Name: "Jinzhu", Age: 18, Role: "Admin", Salary: 100000}, {Name: "jinzhu 2", Age: 30, Role: "Dev", Salary: 60000}}
		employee  = Employee0{Salary: 150000}
		employees = []Employee0{}
	)

	cpy.Copy(&employee, &user)

	fmt.Printf("%#v \n", employee)
	// Employee{
	//    Name: "Jinzhu",           // Copy from field
	//    Age: 18,                  // Copy from field
	//    Salary:150000,            // Copying explicitly ignored
	//    DoubleAge: 36,            // Copy from method
	//    EmployeeId: 0,            // Ignored
	//    SuperRole: "Super Admin", // Copy to method
	// }

	// Copy struct to slice
	cpy.Copy(&employees, &user)

	fmt.Printf("%#v \n", employees)
	// []Employee{
	//   {Name: "Jinzhu", Age: 18, Salary:0, DoubleAge: 36, EmployeId: 0, SuperRole: "Super Admin"}
	// }

	// Copy slice to slice
	employees = []Employee0{}
	cpy.Copy(&employees, &users)

	fmt.Printf("%#v \n", employees)
	// []Employee{
	//   {Name: "Jinzhu", Age: 18, Salary:0, DoubleAge: 36, EmployeId: 0, SuperRole: "Super Admin"},
	//   {Name: "jinzhu 2", Age: 30, Salary:0, DoubleAge: 60, EmployeId: 0, SuperRole: "Super Dev"},
	// }

	// Copy map to map
	map1 := map[int]int{3: 6, 4: 8}
	map2 := map[int32]int8{}
	cpy.Copy(&map2, map1)

	fmt.Printf("%#v \n", map2)
	// map[int32]int8{3:6, 4:8}

	map3 := map[string]any{}
	cpy.Copy(&map3, &user)
	fmt.Printf("%#v \n", map3)
}
